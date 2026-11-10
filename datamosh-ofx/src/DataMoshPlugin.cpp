// DataMosh: an OpenFX image effect for DaVinci Resolve (and other OFX hosts).
//
// Written against the plain OFX C API so the only dependency is the OFX
// headers in third_party/openfx. All the moshing happens in MoshEngine; this
// file fetches frames, caches work between renders and maps parameters.
//
// Hosts render frames in any order (scrubbing, background cache), so a mosh
// cannot simply remember "the last frame". Every render rebuilds the mosh
// from its start frame, but the result of each render is cached, so playing
// forward only ever costs one new step per frame. Motion fields are cached by
// the content of the frames they came from, so they survive scrubbing.
#include <cmath>
#include <cstdint>
#include <cstring>
#include <deque>
#include <list>
#include <memory>
#include <mutex>
#include <string>

#include "MoshEngine.h"
#include "ofxCore.h"
#include "ofxImageEffect.h"
#include "ofxMessage.h"
#include "ofxParam.h"
#include "ofxProperty.h"

#if defined(_WIN32)
#define DM_EXPORT extern "C" __declspec(dllexport)
#else
#define DM_EXPORT extern "C" __attribute__((visibility("default")))
#endif

namespace {

const char* const kPluginId = "com.lumpy.DataMosh";
const char* const kGrouping = "Lumpy";
const int kVersionMajor = 1, kVersionMinor = 0;

// Parameter names (stable: they are saved in projects).
const char* const kParamMode = "mode";
const char* const kParamStart = "moshStart";
const char* const kParamSetStart = "setStartHere";
const char* const kParamLength = "moshLength";
const char* const kParamBlock = "blockSize";
const char* const kParamRange = "searchRange";
const char* const kParamScale = "motionScale";
const char* const kParamResidual = "residual";
const char* const kParamHeal = "heal";
const char* const kParamSmooth = "smoothVectors";
const char* const kParamMix = "mix";

enum Mode { kModeDrag = 0, kModeBloom = 1 };
const int kBlockSizes[] = {8, 16, 32};

OfxHost* gHost = nullptr;
const OfxPropertySuiteV1* gProp = nullptr;
const OfxImageEffectSuiteV1* gFx = nullptr;
const OfxParameterSuiteV1* gParam = nullptr;
const OfxMessageSuiteV1* gMsg = nullptr;

// ---------------------------------------------------------------- instance

struct Params {
    int mode = kModeDrag;
    int start = 0;
    int length = 48;
    int block = 16;
    int range = 24;
    double scale = 1.0;
    double residual = 0.3;
    double heal = 0.0;
    bool smooth = false;
    double mix = 1.0;
};

struct Frame {
    mosh::Image img;
    OfxRectI bounds{0, 0, 0, 0};
    uint64_t fp = 0;
    bool ok = false;
};

struct CachedState {
    uint64_t key = 0;
    int time = 0;
    uint64_t fpAtTime = 0;  // fingerprint of the real frame at `time`
    mosh::Image img;
};

struct Instance {
    OfxImageClipHandle src = nullptr, dst = nullptr;
    OfxParamHandle mode, start, setStart, length, block, range, scale, residual, heal, smooth,
        mix;

    std::mutex lock;
    std::deque<CachedState> states;  // newest first
    std::list<std::pair<uint64_t, mosh::MotionField>> motion;  // LRU, newest first
};

Instance* getInstance(OfxImageEffectHandle effect) {
    OfxPropertySetHandle props;
    gFx->getPropertySet(effect, &props);
    void* p = nullptr;
    gProp->propGetPointer(props, kOfxPropInstanceData, 0, &p);
    return static_cast<Instance*>(p);
}

uint64_t hashMix(uint64_t h, uint64_t v) {
    h ^= v + 0x9e3779b97f4a7c15ull + (h << 6) + (h >> 2);
    return h;
}

uint64_t hashDouble(uint64_t h, double d) {
    uint64_t bits;
    std::memcpy(&bits, &d, sizeof bits);
    return hashMix(h, bits);
}

Params readParams(Instance* in, double t) {
    Params p;
    int i = 0;
    gParam->paramGetValueAtTime(in->mode, t, &p.mode);
    gParam->paramGetValueAtTime(in->start, t, &p.start);
    gParam->paramGetValueAtTime(in->length, t, &p.length);
    gParam->paramGetValueAtTime(in->block, t, &i);
    p.block = kBlockSizes[std::min(std::max(i, 0), 2)];
    gParam->paramGetValueAtTime(in->range, t, &p.range);
    gParam->paramGetValueAtTime(in->scale, t, &p.scale);
    gParam->paramGetValueAtTime(in->residual, t, &p.residual);
    gParam->paramGetValueAtTime(in->heal, t, &p.heal);
    i = 0;
    gParam->paramGetValueAtTime(in->smooth, t, &i);
    p.smooth = i != 0;
    gParam->paramGetValueAtTime(in->mix, t, &p.mix);
    p.length = std::max(1, p.length);
    return p;
}

bool inMosh(const Params& p, int frame) {
    return frame >= p.start && frame < p.start + p.length;
}

// Copy a host RGBA float image into an engine image.
bool fetchFrame(OfxImageClipHandle clip, double t, Frame& f) {
    f = Frame{};
    OfxPropertySetHandle img = nullptr;
    if (gFx->clipGetImage(clip, t, nullptr, &img) != kOfxStatOK || !img) return false;

    void* data = nullptr;
    int rowBytes = 0;
    char* depth = nullptr;
    char* comps = nullptr;
    gProp->propGetPointer(img, kOfxImagePropData, 0, &data);
    gProp->propGetInt(img, kOfxImagePropRowBytes, 0, &rowBytes);
    gProp->propGetIntN(img, kOfxImagePropBounds, 4, &f.bounds.x1);
    gProp->propGetString(img, kOfxImageEffectPropPixelDepth, 0, &depth);
    gProp->propGetString(img, kOfxImageEffectPropComponents, 0, &comps);

    const int w = f.bounds.x2 - f.bounds.x1, h = f.bounds.y2 - f.bounds.y1;
    const bool usable = data && w > 0 && h > 0 && depth && comps &&
                        std::strcmp(depth, kOfxBitDepthFloat) == 0 &&
                        std::strcmp(comps, kOfxImageComponentRGBA) == 0;
    if (usable) {
        f.img = mosh::Image(w, h);
        for (int y = 0; y < h; ++y) {
            const char* row = static_cast<const char*>(data) + std::ptrdiff_t(y) * rowBytes;
            std::memcpy(f.img.row(y), row, size_t(w) * 4 * sizeof(float));
        }
        f.fp = mosh::fingerprint(f.img);
        f.ok = true;
    }
    gFx->clipReleaseImage(img);
    return f.ok;
}

const mosh::MotionField& motionFor(Instance* in, const Frame& cur, const Frame& prev,
                                   const mosh::MotionSettings& ms) {
    uint64_t key = hashMix(hashMix(hashMix(hashMix(1, cur.fp), prev.fp), ms.block), ms.range);
    for (auto it = in->motion.begin(); it != in->motion.end(); ++it) {
        if (it->first == key) {
            in->motion.splice(in->motion.begin(), in->motion, it);
            return in->motion.front().second;
        }
    }
    in->motion.emplace_front(key, mosh::estimateMotion(cur.img, prev.img, ms));
    while (in->motion.size() > 600) in->motion.pop_back();  // ~600 * 65 KB at 1080p/16
    return in->motion.front().second;
}

void storeState(Instance* in, uint64_t key, int time, uint64_t fp, const mosh::Image& img) {
    for (auto it = in->states.begin(); it != in->states.end(); ++it) {
        if (it->key == key && it->time == time) {
            in->states.erase(it);
            break;
        }
    }
    in->states.push_front(CachedState{key, time, fp, img});
    while (in->states.size() > 3) in->states.pop_back();
}

// Latest cached state for `key` with time in [lo, hi], or null.
CachedState* findState(Instance* in, uint64_t key, int lo, int hi) {
    CachedState* best = nullptr;
    for (auto& s : in->states)
        if (s.key == key && s.time >= lo && s.time <= hi && (!best || s.time > best->time))
            best = &s;
    return best;
}

uint64_t settingsKey(const Params& p, const mosh::MotionSettings& ms,
                     const mosh::StepSettings& ss) {
    uint64_t k = 7;
    k = hashMix(k, uint64_t(p.mode));
    k = hashMix(k, uint64_t(int64_t(p.start)));
    k = hashMix(k, uint64_t(ms.block));
    k = hashMix(k, uint64_t(ms.range));
    k = hashDouble(k, ss.motionScale);
    k = hashDouble(k, ss.residual);
    k = hashDouble(k, ss.heal);
    k = hashMix(k, ss.smooth ? 1 : 0);
    return k;
}

// Builds the moshed picture for frame t. On success `out` holds it and `real`
// holds the untouched source frame at t (for Mix).
bool buildMosh(OfxImageEffectHandle effect, Instance* in, const Params& p, int t,
               double renderScale, mosh::Image& out, Frame& real) {
    mosh::MotionSettings ms;
    ms.block = std::max(4, int(std::lround(p.block * renderScale)));
    ms.range = std::max(2, int(std::lround(p.range * renderScale)));
    mosh::StepSettings ss;
    ss.motionScale = float(p.scale);
    ss.residual = float(p.residual);
    ss.heal = float(p.heal);
    ss.smooth = p.smooth;

    const uint64_t base = settingsKey(p, ms, ss);
    mosh::Image a, b;

    if (p.mode == kModeBloom) {
        // Freeze frame `start` and keep re-applying its motion.
        Frame frozen, before;
        if (!fetchFrame(in->src, p.start, frozen)) return false;
        const bool haveBefore = fetchFrame(in->src, p.start - 1, before) &&
                                before.img.w == frozen.img.w && before.img.h == frozen.img.h;
        mosh::MotionField none;
        const mosh::MotionField& mf = haveBefore ? motionFor(in, frozen, before, ms) : none;
        const uint64_t key = hashMix(hashMix(base, frozen.fp), haveBefore ? before.fp : 0);

        int from = p.start;
        mosh::Image state = frozen.img;
        if (CachedState* c = findState(in, key, p.start, t)) {
            from = c->time;
            state = c->img;
        }
        for (int k = from + 1; k <= t; ++k) {
            Frame cur;
            if (ss.heal > 0.f) fetchFrame(in->src, k, cur);
            mosh::stepBloom(state, cur.img, mf, ss, a);
            if (gFx->abort(effect)) return false;
        }
        if (!fetchFrame(in->src, t, real)) real = frozen;
        storeState(in, key, t, real.fp, state);
        out = std::move(state);
        return true;
    }

    // Drag: frame `start` is the dropped I-frame, so it shows the frame
    // before it; every later frame applies its own motion and residual.
    Frame ref;
    if (!fetchFrame(in->src, p.start - 1, ref) && !fetchFrame(in->src, p.start, ref))
        return false;
    const uint64_t key = hashMix(base, ref.fp);

    int from = p.start;
    mosh::Image state = ref.img;
    Frame prev;
    bool resumed = false;
    if (CachedState* c = findState(in, key, p.start, t)) {
        // Only trust the cache if the footage at that frame is unchanged.
        if (fetchFrame(in->src, c->time, prev) && prev.fp == c->fpAtTime) {
            from = c->time;
            state = c->img;
            resumed = true;
        } else {
            in->states.clear();
        }
    }
    if (!resumed && !fetchFrame(in->src, p.start, prev)) prev = ref;

    for (int k = from + 1; k <= t; ++k) {
        Frame cur;
        if (!fetchFrame(in->src, k, cur)) break;  // ran off the end of the clip
        if (cur.img.w != state.w || cur.img.h != state.h || prev.img.w != cur.img.w ||
            prev.img.h != cur.img.h) {
            prev = std::move(cur);
            continue;
        }
        const mosh::MotionField& mf = motionFor(in, cur, prev, ms);
        mosh::stepDrag(state, cur.img, prev.img, mf, ss, a, b);
        prev = std::move(cur);
        if (gFx->abort(effect)) return false;
    }
    real = std::move(prev);
    storeState(in, key, t, real.fp, state);
    out = std::move(state);
    return true;
}

// ---------------------------------------------------------------- actions

OfxStatus onLoad() {
    if (!gHost) return kOfxStatErrMissingHostFeature;
    gProp = static_cast<const OfxPropertySuiteV1*>(gHost->fetchSuite(gHost->host, kOfxPropertySuite, 1));
    gFx = static_cast<const OfxImageEffectSuiteV1*>(gHost->fetchSuite(gHost->host, kOfxImageEffectSuite, 1));
    gParam = static_cast<const OfxParameterSuiteV1*>(gHost->fetchSuite(gHost->host, kOfxParameterSuite, 1));
    gMsg = static_cast<const OfxMessageSuiteV1*>(gHost->fetchSuite(gHost->host, kOfxMessageSuite, 1));
    return (gProp && gFx && gParam) ? kOfxStatOK : kOfxStatErrMissingHostFeature;
}

OfxStatus describe(OfxImageEffectHandle effect) {
    OfxPropertySetHandle p;
    gFx->getPropertySet(effect, &p);
    gProp->propSetString(p, kOfxPropLabel, 0, "DataMosh");
    gProp->propSetString(p, kOfxImageEffectPluginPropGrouping, 0, kGrouping);
    gProp->propSetString(p, kOfxPropPluginDescription, 0,
                         "Simulated datamosh: drops the I-frame at Mosh Start so the next "
                         "shot's motion drags the previous picture around (Drag), or repeats "
                         "one frame's motion over and over (Bloom).");
    gProp->propSetString(p, kOfxImageEffectPropSupportedContexts, 0, kOfxImageEffectContextFilter);
    gProp->propSetString(p, kOfxImageEffectPropSupportedContexts, 1, kOfxImageEffectContextGeneral);
    gProp->propSetString(p, kOfxImageEffectPropSupportedPixelDepths, 0, kOfxBitDepthFloat);
    gProp->propSetInt(p, kOfxImageEffectPropSupportsTiles, 0, 0);
    gProp->propSetInt(p, kOfxImageEffectPropTemporalClipAccess, 0, 1);
    gProp->propSetInt(p, kOfxImageEffectPropSupportsMultiResolution, 0, 1);
    gProp->propSetInt(p, kOfxImageEffectPropSupportsMultipleClipDepths, 0, 0);
    gProp->propSetInt(p, kOfxImageEffectPluginPropHostFrameThreading, 0, 0);
    gProp->propSetString(p, kOfxImageEffectPluginRenderThreadSafety, 0,
                         kOfxImageEffectRenderInstanceSafe);
    return kOfxStatOK;
}

void setLabelHint(OfxPropertySetHandle p, const char* label, const char* hint) {
    gProp->propSetString(p, kOfxPropLabel, 0, label);
    gProp->propSetString(p, kOfxParamPropHint, 0, hint);
}

void defineInt(OfxParamSetHandle ps, const char* name, const char* label, const char* hint,
               int def, int lo, int hi, int dlo, int dhi) {
    OfxPropertySetHandle p;
    gParam->paramDefine(ps, kOfxParamTypeInteger, name, &p);
    setLabelHint(p, label, hint);
    gProp->propSetInt(p, kOfxParamPropDefault, 0, def);
    gProp->propSetInt(p, kOfxParamPropMin, 0, lo);
    gProp->propSetInt(p, kOfxParamPropMax, 0, hi);
    gProp->propSetInt(p, kOfxParamPropDisplayMin, 0, dlo);
    gProp->propSetInt(p, kOfxParamPropDisplayMax, 0, dhi);
    gProp->propSetInt(p, kOfxParamPropAnimates, 0, 0);
}

void defineDouble(OfxParamSetHandle ps, const char* name, const char* label, const char* hint,
                  double def, double lo, double hi, bool animates) {
    OfxPropertySetHandle p;
    gParam->paramDefine(ps, kOfxParamTypeDouble, name, &p);
    setLabelHint(p, label, hint);
    gProp->propSetDouble(p, kOfxParamPropDefault, 0, def);
    gProp->propSetDouble(p, kOfxParamPropMin, 0, lo);
    gProp->propSetDouble(p, kOfxParamPropMax, 0, hi);
    gProp->propSetDouble(p, kOfxParamPropDisplayMin, 0, lo);
    gProp->propSetDouble(p, kOfxParamPropDisplayMax, 0, hi);
    gProp->propSetDouble(p, kOfxParamPropIncrement, 0, 0.01);
    gProp->propSetInt(p, kOfxParamPropDigits, 0, 2);
    gProp->propSetInt(p, kOfxParamPropAnimates, 0, animates ? 1 : 0);
}

OfxStatus describeInContext(OfxImageEffectHandle effect, OfxPropertySetHandle inArgs) {
    char* context = nullptr;
    gProp->propGetString(inArgs, kOfxImageEffectPropContext, 0, &context);

    OfxPropertySetHandle clip;
    gFx->clipDefine(effect, kOfxImageEffectSimpleSourceClipName, &clip);
    gProp->propSetString(clip, kOfxImageEffectPropSupportedComponents, 0, kOfxImageComponentRGBA);
    gProp->propSetInt(clip, kOfxImageClipPropIsMask, 0, 0);
    gProp->propSetInt(clip, kOfxImageEffectPropTemporalClipAccess, 0, 1);
    gProp->propSetInt(clip, kOfxImageEffectPropSupportsTiles, 0, 0);

    gFx->clipDefine(effect, kOfxImageEffectOutputClipName, &clip);
    gProp->propSetString(clip, kOfxImageEffectPropSupportedComponents, 0, kOfxImageComponentRGBA);
    gProp->propSetInt(clip, kOfxImageEffectPropSupportsTiles, 0, 0);

    OfxParamSetHandle ps;
    gFx->getParamSet(effect, &ps);
    OfxPropertySetHandle p;

    gParam->paramDefine(ps, kOfxParamTypeChoice, kParamMode, &p);
    setLabelHint(p, "Mode",
                 "Drag: the frame before Mosh Start is held and the new shot's motion pushes "
                 "it around (deleted I-frame). Bloom: Mosh Start's motion is repeated over and "
                 "over (duplicated P-frame).");
    gProp->propSetString(p, kOfxParamPropChoiceOption, 0, "Drag (drop I-frame)");
    gProp->propSetString(p, kOfxParamPropChoiceOption, 1, "Bloom (repeat P-frame)");
    gProp->propSetInt(p, kOfxParamPropDefault, 0, kModeDrag);
    gProp->propSetInt(p, kOfxParamPropAnimates, 0, 0);

    defineInt(ps, kParamStart, "Mosh Start Frame",
              "First frame of the mosh, usually the first frame of the new shot. Easiest to "
              "set with \"Start Mosh Here\".",
              0, -1000000, 1000000, 0, 1000);

    gParam->paramDefine(ps, kOfxParamTypePushButton, kParamSetStart, &p);
    setLabelHint(p, "Start Mosh Here", "Set Mosh Start Frame to the current frame.");

    defineInt(ps, kParamLength, "Mosh Length",
              "How many frames the mosh lasts before the picture cleans up (a fresh I-frame).",
              48, 1, 2000, 1, 240);

    gParam->paramDefine(ps, kOfxParamTypeChoice, kParamBlock, &p);
    setLabelHint(p, "Block Size",
                 "Macroblock size. Smaller blocks follow motion more closely; bigger blocks "
                 "are chunkier and more glitchy.");
    gProp->propSetString(p, kOfxParamPropChoiceOption, 0, "8 px");
    gProp->propSetString(p, kOfxParamPropChoiceOption, 1, "16 px");
    gProp->propSetString(p, kOfxParamPropChoiceOption, 2, "32 px");
    gProp->propSetInt(p, kOfxParamPropDefault, 0, 1);
    gProp->propSetInt(p, kOfxParamPropAnimates, 0, 0);

    defineInt(ps, kParamRange, "Search Range",
              "How far (pixels) to look for each block's motion. Raise it for fast motion; "
              "costs render time.",
              24, 2, 128, 4, 64);
    defineDouble(ps, kParamScale, "Motion Scale",
                 "Multiplies every motion vector. Above 1 smears faster than the real motion.",
                 1.0, 0.0, 4.0, false);
    defineDouble(ps, kParamResidual, "Residual Bleed",
                 "How much of the new shot's detail leaks through. 0 = pure smear of the old "
                 "picture, 1 = what a real decoder would do.",
                 0.3, 0.0, 1.0, false);
    defineDouble(ps, kParamHeal, "Heal",
                 "Per-frame blend back toward the real picture, so the mosh slowly resolves.",
                 0.0, 0.0, 1.0, false);

    gParam->paramDefine(ps, kOfxParamTypeBoolean, kParamSmooth, &p);
    setLabelHint(p, "Smooth Vectors",
                 "Interpolate motion between blocks for a liquid smear instead of blocky "
                 "macroblock tearing.");
    gProp->propSetInt(p, kOfxParamPropDefault, 0, 0);
    gProp->propSetInt(p, kOfxParamPropAnimates, 0, 0);

    defineDouble(ps, kParamMix, "Mix", "Blend between the original (0) and the mosh (1).", 1.0,
                 0.0, 1.0, true);
    (void)context;
    return kOfxStatOK;
}

OfxStatus createInstance(OfxImageEffectHandle effect) {
    auto* in = new Instance;
    gFx->clipGetHandle(effect, kOfxImageEffectSimpleSourceClipName, &in->src, nullptr);
    gFx->clipGetHandle(effect, kOfxImageEffectOutputClipName, &in->dst, nullptr);
    OfxParamSetHandle ps;
    gFx->getParamSet(effect, &ps);
    gParam->paramGetHandle(ps, kParamMode, &in->mode, nullptr);
    gParam->paramGetHandle(ps, kParamStart, &in->start, nullptr);
    gParam->paramGetHandle(ps, kParamSetStart, &in->setStart, nullptr);
    gParam->paramGetHandle(ps, kParamLength, &in->length, nullptr);
    gParam->paramGetHandle(ps, kParamBlock, &in->block, nullptr);
    gParam->paramGetHandle(ps, kParamRange, &in->range, nullptr);
    gParam->paramGetHandle(ps, kParamScale, &in->scale, nullptr);
    gParam->paramGetHandle(ps, kParamResidual, &in->residual, nullptr);
    gParam->paramGetHandle(ps, kParamHeal, &in->heal, nullptr);
    gParam->paramGetHandle(ps, kParamSmooth, &in->smooth, nullptr);
    gParam->paramGetHandle(ps, kParamMix, &in->mix, nullptr);

    OfxPropertySetHandle props;
    gFx->getPropertySet(effect, &props);
    gProp->propSetPointer(props, kOfxPropInstanceData, 0, in);
    return kOfxStatOK;
}

OfxStatus destroyInstance(OfxImageEffectHandle effect) {
    delete getInstance(effect);
    OfxPropertySetHandle props;
    gFx->getPropertySet(effect, &props);
    gProp->propSetPointer(props, kOfxPropInstanceData, 0, nullptr);
    return kOfxStatOK;
}

OfxStatus instanceChanged(OfxImageEffectHandle effect, OfxPropertySetHandle inArgs) {
    char* type = nullptr;
    char* name = nullptr;
    gProp->propGetString(inArgs, kOfxPropType, 0, &type);
    gProp->propGetString(inArgs, kOfxPropName, 0, &name);
    if (!type || !name || std::strcmp(type, kOfxTypeParameter) != 0) return kOfxStatReplyDefault;
    if (std::strcmp(name, kParamSetStart) == 0) {
        double t = 0;
        gProp->propGetDouble(inArgs, kOfxPropTime, 0, &t);
        Instance* in = getInstance(effect);
        if (in) gParam->paramSetValue(in->start, int(std::lround(t)));
        return kOfxStatOK;
    }
    return kOfxStatReplyDefault;
}

OfxStatus isIdentity(OfxImageEffectHandle effect, OfxPropertySetHandle inArgs,
                     OfxPropertySetHandle outArgs) {
    Instance* in = getInstance(effect);
    double t = 0;
    gProp->propGetDouble(inArgs, kOfxPropTime, 0, &t);
    Params p = readParams(in, t);
    if (inMosh(p, int(std::lround(t))) && p.mix > 0.0) return kOfxStatReplyDefault;
    gProp->propSetString(outArgs, kOfxPropName, 0, kOfxImageEffectSimpleSourceClipName);
    gProp->propSetDouble(outArgs, kOfxPropTime, 0, t);
    return kOfxStatOK;
}

OfxStatus getFramesNeeded(OfxImageEffectHandle effect, OfxPropertySetHandle inArgs,
                          OfxPropertySetHandle outArgs) {
    Instance* in = getInstance(effect);
    double t = 0;
    gProp->propGetDouble(inArgs, kOfxPropTime, 0, &t);
    Params p = readParams(in, t);
    double range[2] = {t, t};
    if (inMosh(p, int(std::lround(t)))) range[0] = std::min<double>(p.start - 1, t);
    std::string prop = std::string("OfxImageClipPropFrameRange_") +
                       kOfxImageEffectSimpleSourceClipName;
    gProp->propSetDoubleN(outArgs, prop.c_str(), 2, range);
    return kOfxStatOK;
}

OfxStatus render(OfxImageEffectHandle effect, OfxPropertySetHandle inArgs) {
    Instance* in = getInstance(effect);
    if (!in) return kOfxStatErrBadHandle;
    double t = 0;
    OfxRectI win{0, 0, 0, 0};
    double rs[2] = {1, 1};
    gProp->propGetDouble(inArgs, kOfxPropTime, 0, &t);
    gProp->propGetIntN(inArgs, kOfxImageEffectPropRenderWindow, 4, &win.x1);
    gProp->propGetDoubleN(inArgs, kOfxImageEffectPropRenderScale, 2, rs);
    const int frame = int(std::lround(t));
    Params p = readParams(in, t);

    OfxPropertySetHandle dstImg = nullptr;
    if (gFx->clipGetImage(in->dst, t, nullptr, &dstImg) != kOfxStatOK || !dstImg)
        return kOfxStatFailed;
    void* dstData = nullptr;
    int dstRowBytes = 0;
    OfxRectI db;
    gProp->propGetPointer(dstImg, kOfxImagePropData, 0, &dstData);
    gProp->propGetInt(dstImg, kOfxImagePropRowBytes, 0, &dstRowBytes);
    gProp->propGetIntN(dstImg, kOfxImagePropBounds, 4, &db.x1);

    mosh::Image moshed;
    Frame real;
    bool haveMosh = false;
    {
        std::lock_guard<std::mutex> g(in->lock);
        if (inMosh(p, frame) && p.mix > 0.0)
            haveMosh = buildMosh(effect, in, p, frame, rs[0] > 0 ? rs[0] : 1.0, moshed, real);
    }
    if (!haveMosh && !real.ok) fetchFrame(in->src, t, real);
    if (!real.ok && !haveMosh) {
        gFx->clipReleaseImage(dstImg);
        return gFx->abort(effect) ? kOfxStatOK : kOfxStatFailed;
    }

    const mosh::Image& m = haveMosh ? moshed : real.img;
    const OfxRectI sb = real.ok ? real.bounds : OfxRectI{db.x1, db.y1, db.x1 + m.w, db.y1 + m.h};
    const float mix = haveMosh ? float(p.mix) : 0.f;

    const int x1 = std::max(win.x1, db.x1), x2 = std::min(win.x2, db.x2);
    const int y1 = std::max(win.y1, db.y1), y2 = std::min(win.y2, db.y2);
    mosh::parallelFor(std::max(0, y2 - y1), [&](int j) {
        const int y = y1 + j;
        float* d = reinterpret_cast<float*>(static_cast<char*>(dstData) +
                                            std::ptrdiff_t(y - db.y1) * dstRowBytes) +
                   size_t(x1 - db.x1) * 4;
        const int sy = y - sb.y1;
        for (int x = x1; x < x2; ++x, d += 4) {
            const int sx = x - sb.x1;
            const bool inM = sy >= 0 && sy < m.h && sx >= 0 && sx < m.w;
            const bool inR = real.ok && sy >= 0 && sy < real.img.h && sx >= 0 && sx < real.img.w;
            const float* mp = inM ? m.row(sy) + size_t(sx) * 4 : nullptr;
            const float* rp = inR ? real.img.row(sy) + size_t(sx) * 4 : nullptr;
            for (int k = 0; k < 4; ++k) {
                float mv = mp ? mp[k] : 0.f;
                float rv = rp ? rp[k] : mv;
                d[k] = rv + (mv - rv) * mix;
            }
        }
    });

    gFx->clipReleaseImage(dstImg);
    return kOfxStatOK;
}

OfxStatus mainEntry(const char* action, const void* handle, OfxPropertySetHandle inArgs,
                    OfxPropertySetHandle outArgs) {
    try {
        auto effect = (OfxImageEffectHandle)handle;
        if (std::strcmp(action, kOfxActionLoad) == 0) return onLoad();
        if (std::strcmp(action, kOfxActionUnload) == 0) return kOfxStatOK;
        if (std::strcmp(action, kOfxActionDescribe) == 0) return describe(effect);
        if (std::strcmp(action, kOfxImageEffectActionDescribeInContext) == 0)
            return describeInContext(effect, inArgs);
        if (std::strcmp(action, kOfxActionCreateInstance) == 0) return createInstance(effect);
        if (std::strcmp(action, kOfxActionDestroyInstance) == 0) return destroyInstance(effect);
        if (std::strcmp(action, kOfxActionInstanceChanged) == 0)
            return instanceChanged(effect, inArgs);
        if (std::strcmp(action, kOfxImageEffectActionIsIdentity) == 0)
            return isIdentity(effect, inArgs, outArgs);
        if (std::strcmp(action, kOfxImageEffectActionGetFramesNeeded) == 0)
            return getFramesNeeded(effect, inArgs, outArgs);
        if (std::strcmp(action, kOfxImageEffectActionRender) == 0) return render(effect, inArgs);
        if (std::strcmp(action, kOfxActionPurgeCaches) == 0) {
            if (Instance* in = getInstance(effect)) {
                std::lock_guard<std::mutex> g(in->lock);
                in->states.clear();
                in->motion.clear();
            }
            return kOfxStatOK;
        }
        return kOfxStatReplyDefault;
    } catch (const std::bad_alloc&) {
        return kOfxStatErrMemory;
    } catch (...) {
        return kOfxStatFailed;
    }
}

void setHost(OfxHost* host) { gHost = host; }

OfxPlugin gPlugin = {kOfxImageEffectPluginApi, 1, kPluginId, kVersionMajor, kVersionMinor,
                     setHost, mainEntry};

}  // namespace

DM_EXPORT int OfxGetNumberOfPlugins(void) { return 1; }

DM_EXPORT OfxPlugin* OfxGetPlugin(int nth) { return nth == 0 ? &gPlugin : nullptr; }
