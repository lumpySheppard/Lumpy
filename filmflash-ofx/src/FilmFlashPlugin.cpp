// FilmFlash: an OpenFX transition / effect for DaVinci Resolve that recreates
// the exposure flash of a film camera running down and back up to speed.
//
// As a transition (drag it onto a cut): the outgoing shot builds to a flash as
// the camera "slows", the cut happens at the peak, and the incoming shot
// clears as the camera gets back up to speed. As an effect: keyframe Flash
// (stops) yourself.
//
// Plain OFX C API; the image work is in FlashEngine.
#include <algorithm>
#include <cmath>
#include <cstdint>
#include <cstring>
#include <string>
#include <vector>

#include "FlashEngine.h"
#include "ofxCore.h"
#include "ofxImageEffect.h"
#include "ofxParam.h"
#include "ofxProperty.h"

#if defined(_WIN32)
#define FF_EXPORT extern "C" __declspec(dllexport)
#else
#define FF_EXPORT extern "C" __attribute__((visibility("default")))
#endif

namespace {

const char* const kPluginId = "com.lumpy.FilmFlash";
const char* const kGrouping = "Lumpy";

enum P {
    P_FLASH,  // effect only
    P_PEAK, P_CUT, P_HOLD, P_RUNOUT, P_RUNUP, P_SOFT,  // transition only
    P_FLICKER, P_SEED,
    P_ENCODING, P_RESPONSE, P_SHIFT, P_HICOL, P_FOG, P_FOGCOL, P_HAL, P_HALSIZE,
    P_UNEVEN, P_ANGLE, P_LEAK, P_LEAKCOL, P_CEILING,
    P_SPEEDUP, P_MAXSPEED, P_BLUR, P_SHUTTER, P_MAXBLUR,
    P_COUNT
};
enum PType { kDbl, kInt, kChoice, kRGB };
enum Ctx { kBoth, kFilterOnly, kTransitionOnly };

struct PDef {
    P id;
    const char* name;
    PType type;
    Ctx ctx;
    const char* group;
    const char* label;
    const char* hint;
    double def[3];
    double lo, hi, dlo, dhi;
    std::vector<const char*> options;
};

struct GDef {
    const char* name;
    const char* label;
    bool open;
};
const GDef kGroups[] = {{"grpSpeed", "Camera Speed", true},
                        {"grpFilm", "Film", true},
                        {"grpCamera", "Camera & Light", false}};

const std::vector<PDef>& defs() {
    static const std::vector<PDef> d = {
        {P_FLASH, "flash", kDbl, kFilterOnly, nullptr, "Flash (stops)",
         "Extra exposure in stops. Keyframe it to build and release the flash.", {0}, -4, 14, 0,
         10, {}},
        {P_PEAK, "peak", kDbl, kTransitionOnly, nullptr, "Peak (stops)",
         "How far over the flash goes at the cut. 6-8 stops burns nearly to white.", {8}, 0, 14,
         0, 12, {}},
        {P_CUT, "cutPoint", kDbl, kTransitionOnly, nullptr, "Cut Point",
         "Where in the transition the outgoing shot hands over to the incoming one (and the "
         "flash peaks).",
         {0.5}, 0, 1, 0, 1, {}},
        {P_HOLD, "hold", kDbl, kTransitionOnly, nullptr, "Peak Hold",
         "Share of the transition held at the peak, around the cut.", {0.08}, 0, 0.8, 0, 0.5, {}},
        {P_RUNOUT, "runOut", kDbl, kTransitionOnly, nullptr, "Run-out Shape",
         "How the outgoing shot builds. 1 = a motor slowing evenly (creeps up, then shoots "
         "off). Higher = holds off longer, then rushes.",
         {1}, 0.1, 8, 0.25, 4, {}},
        {P_RUNUP, "runUp", kDbl, kTransitionOnly, nullptr, "Run-up Shape",
         "How fast the incoming shot clears. Higher = the camera gets back up to speed faster.",
         {2}, 0.1, 8, 0.25, 6, {}},
        {P_SOFT, "cutSoftness", kDbl, kTransitionOnly, nullptr, "Cut Softness",
         "0 = a hard cut at the peak (hidden by the flash). Higher = a short dissolve across "
         "it.",
         {0}, 0, 0.5, 0, 0.3, {}},
        {P_FLICKER, "flicker", kDbl, kBoth, nullptr, "Flicker",
         "Random frame-to-frame exposure wobble while flashing, in stops (an unsteady motor).",
         {0.2}, 0, 3, 0, 1, {}},
        {P_SEED, "seed", kInt, kBoth, nullptr, "Seed", "Changes the flicker pattern.", {1}, 0,
         1000000, 0, 100, {}},

        {P_SPEEDUP, "speedUp", kDbl, kTransitionOnly, "grpSpeed", "Speed Up",
         "The action speeds up as the camera slows (a camera at 1/4 speed plays back 4x "
         "fast). 1 = tied to the exposure like a real camera, 0 = off. Reads further into "
         "the clips' handles.",
         {1}, 0, 2, 0, 1, {}},
        {P_MAXSPEED, "maxSpeed", kDbl, kTransitionOnly, "grpSpeed", "Max Speed",
         "Cap on how fast the action runs near the peak.", {6}, 1, 32, 1, 12, {}},
        {P_BLUR, "motionBlur", kDbl, kBoth, "grpSpeed", "Motion Blur",
         "Longer exposures smear more motion: frames are averaged across the longer shutter "
         "time. 1 = tied to the exposure, 0 = off.",
         {1}, 0, 2, 0, 1, {}},
        {P_SHUTTER, "shutterAngle", kDbl, kBoth, "grpSpeed", "Shutter Angle",
         "The camera's shutter angle at normal speed (180 is standard).", {180}, 5, 360, 45,
         360, {}},
        {P_MAXBLUR, "maxBlur", kDbl, kBoth, "grpSpeed", "Max Blur",
         "Cap on the blur, in frames of action. More costs render time.", {6}, 0, 32, 0, 12,
         {}},

        {P_ENCODING, "encoding", kChoice, kBoth, "grpFilm", "Input Encoding",
         "How the incoming images are encoded, so stops are real stops. Rec.709 (gamma 2.4) "
         "for a standard timeline; DaVinci Intermediate for a DaVinci Wide Gamut timeline.",
         {0}, 0, 6, 0, 6,
         {"Rec.709 / Gamma 2.4", "sRGB", "Gamma 2.2", "Linear (display)", "Linear (scene)",
          "DaVinci Intermediate", "Cineon Log"}},
        {P_RESPONSE, "response", kDbl, kBoth, "grpFilm", "Film Response",
         "Contrast of the stock's response: how quickly each stop of flash eats into the "
         "picture.",
         {1}, 0.3, 2, 0.5, 1.5, {}},
        {P_SHIFT, "colourShift", kDbl, kBoth, "grpFilm", "Colour Shift",
         "The colour layers differ in speed, so the flash drifts warm as it builds.", {0.5}, 0,
         2, 0, 1, {}},
        {P_HICOL, "highlightColour", kRGB, kBoth, "grpFilm", "Highlight Colour",
         "The white the flash burns toward (the stock and print's clear-base colour).",
         {1, 0.95, 0.84}, 0, 1, 0, 1, {}},
        {P_FOG, "fog", kDbl, kBoth, "grpFilm", "Base Fog",
         "Stray light fogging the base: a milky, coloured lift of the shadows.", {0.35}, 0, 2,
         0, 1, {}},
        {P_FOGCOL, "fogColour", kRGB, kBoth, "grpFilm", "Fog Colour", "Colour of the base fog.",
         {1, 0.62, 0.32}, 0, 1, 0, 1, {}},
        {P_HAL, "halation", kDbl, kBoth, "grpFilm", "Halation",
         "Red glow bleeding around bright areas as they overexpose.", {0.4}, 0, 3, 0, 1, {}},
        {P_HALSIZE, "halationSize", kDbl, kBoth, "grpFilm", "Halation Size",
         "Glow radius, as a percentage of the frame width.", {1}, 0, 10, 0, 4, {}},
        {P_CEILING, "sceneCeiling", kDbl, kBoth, "grpFilm", "Scene Ceiling",
         "Scene-referred encodings only: stops above mid grey where the flash tops out.", {6},
         2, 12, 3, 10, {}},

        {P_UNEVEN, "unevenness", kDbl, kBoth, "grpCamera", "Uneven Exposure",
         "Extra stops on one side of the frame than the other (the shutter at the wrong "
         "speed).",
         {0.6}, 0, 4, 0, 2, {}},
        {P_ANGLE, "direction", kDbl, kBoth, "grpCamera", "Direction",
         "Which side gets more light, in degrees: 0 = left, 90 = top, 180 = right, 270 = "
         "bottom.",
         {0}, -360, 360, 0, 360, {}},
        {P_LEAK, "leak", kDbl, kBoth, "grpCamera", "Edge Leak",
         "Light leaking in at that edge as the camera stops.", {0.3}, 0, 3, 0, 1, {}},
        {P_LEAKCOL, "leakColour", kRGB, kBoth, "grpCamera", "Leak Colour",
         "Colour of the light leak.", {1, 0.35, 0.1}, 0, 1, 0, 1, {}},
    };
    return d;
}

const OfxPropertySuiteV1* gProp = nullptr;
const OfxImageEffectSuiteV1* gFx = nullptr;
const OfxParameterSuiteV1* gParam = nullptr;
OfxHost* gHost = nullptr;

struct Instance {
    bool transition = false;
    OfxImageClipHandle src = nullptr, from = nullptr, to = nullptr, dst = nullptr;
    OfxParamHandle h[P_COUNT] = {};
    OfxParamHandle transitionParam = nullptr;
};

struct Values {
    double v[P_COUNT][3] = {};
    float f(P p, int i = 0) const { return float(v[p][i]); }
};

Instance* getInstance(OfxImageEffectHandle e) {
    OfxPropertySetHandle props;
    gFx->getPropertySet(e, &props);
    void* p = nullptr;
    gProp->propGetPointer(props, kOfxPropInstanceData, 0, &p);
    return static_cast<Instance*>(p);
}

Values readValues(Instance* in, double t) {
    Values vals;
    for (const PDef& d : defs()) {
        for (int i = 0; i < 3; ++i) vals.v[d.id][i] = d.def[i];
        OfxParamHandle h = in->h[d.id];
        if (!h) continue;
        if (d.type == kDbl) {
            gParam->paramGetValueAtTime(h, t, &vals.v[d.id][0]);
        } else if (d.type == kRGB) {
            gParam->paramGetValueAtTime(h, t, &vals.v[d.id][0], &vals.v[d.id][1], &vals.v[d.id][2]);
        } else {
            int x = int(d.def[0]);
            if (gParam->paramGetValueAtTime(h, t, &x) == kOfxStatOK) vals.v[d.id][0] = x;
        }
    }
    return vals;
}

flash::Look lookFrom(const Values& v) {
    flash::Look lk;
    lk.encoding = std::min(std::max(int(v.v[P_ENCODING][0]), 0), 6);
    lk.response = v.f(P_RESPONSE);
    lk.colourShift = v.f(P_SHIFT);
    for (int i = 0; i < 3; ++i) {
        lk.highlightColour[i] = v.f(P_HICOL, i);
        lk.fogColour[i] = v.f(P_FOGCOL, i);
        lk.leakColour[i] = v.f(P_LEAKCOL, i);
    }
    lk.fog = v.f(P_FOG);
    lk.halation = v.f(P_HAL);
    lk.unevenness = v.f(P_UNEVEN);
    lk.angle = v.f(P_ANGLE);
    lk.leak = v.f(P_LEAK);
    lk.sceneCeiling = v.f(P_CEILING);
    lk.upSign = +1;  // OFX images are bottom-up
    return lk;
}

uint64_t mix64(uint64_t x) {
    x += 0x9e3779b97f4a7c15ull;
    x = (x ^ (x >> 30)) * 0xbf58476d1ce4e5b9ull;
    x = (x ^ (x >> 27)) * 0x94d049bb133111ebull;
    return x ^ (x >> 31);
}

// What frame t needs: the flash in stops, and how much of the incoming shot
// to show (0 = all outgoing / the single source, 1 = all incoming).
struct Plan {
    float stops = 0.f;
    float toWeight = 0.f;
    double sFrom = 0, sTo = 0;  // source times (speed-up), sFrom is also the effect's
    float blur = 0.f;           // extra exposure window, source frames
    bool identity(double t) const {
        return stops == 0.f && blur == 0.f && std::fabs(sFrom - t) < 1e-9 && std::fabs(sTo - t) < 1e-9 &&
               (toWeight == 0.f || toWeight == 1.f);
    }
};

flash::Speed speedFrom(const Values& v) {
    flash::Speed sp;
    sp.speedUp = v.f(P_SPEEDUP);
    sp.maxSpeed = v.f(P_MAXSPEED);
    sp.blur = v.f(P_BLUR);
    sp.shutter = v.f(P_SHUTTER);
    sp.maxBlur = v.f(P_MAXBLUR);
    return sp;
}

Plan planFor(Instance* in, const Values& v, double t) {
    Plan pl;
    pl.sFrom = pl.sTo = t;
    const int frame = int(std::lround(t));
    const flash::Speed sp = speedFrom(v);
    if (in->transition) {
        double p = 0, pn = 0, pp = 0;
        if (in->transitionParam) {
            gParam->paramGetValueAtTime(in->transitionParam, t, &p);
            gParam->paramGetValueAtTime(in->transitionParam, t + 1, &pn);
            gParam->paramGetValueAtTime(in->transitionParam, t - 1, &pp);
        }
        flash::Envelope e;
        e.peak = v.f(P_PEAK);
        e.cut = v.f(P_CUT);
        e.hold = v.f(P_HOLD);
        e.runOut = v.f(P_RUNOUT);
        e.runUp = v.f(P_RUNUP);
        e.flicker = v.f(P_FLICKER);
        e.seed = uint32_t(v.v[P_SEED][0]);
        pl.stops = flash::envelopeStops(e, float(p), frame);
        pl.blur = flash::blurFrames(sp, pl.stops);

        // Speed-up: each output frame advances the action by speedFactor
        // source frames. The outgoing shot is pinned at the transition's
        // start and runs ahead; the incoming shot is pinned at its end and
        // starts behind. Needs the transition's frame step, from how the
        // host animates the Transition param.
        double dp = pn - p;
        if (!(dp > 1e-6)) dp = p - pp;
        if (sp.speedUp > 0.f && dp > 1e-6) {
            flash::Envelope e0 = e;
            e0.flicker = 0.f;  // timing follows the motor, not the flicker
            const int K = std::min(20000, int(std::floor(p / dp + 1e-6)));
            for (int k = 1; k <= K; ++k)
                pl.sFrom += flash::speedFactor(sp, flash::envelopeStops(e0, float(p - k * dp), 0)) - 1.0;
            const int K2 = std::min(20000, int(std::floor((1.0 - p) / dp + 1e-6)));
            for (int k = 0; k < K2; ++k)
                pl.sTo -= flash::speedFactor(sp, flash::envelopeStops(e0, float(p + k * dp), 0)) - 1.0;
        }
        const float s = v.f(P_SOFT), c = v.f(P_CUT);
        if (s <= 0.f) {
            pl.toWeight = p >= c ? 1.f : 0.f;
        } else {
            float x = std::min(std::max((float(p) - (c - s / 2)) / s, 0.f), 1.f);
            pl.toWeight = x * x * (3.f - 2.f * x);
        }
    } else {
        float stops = v.f(P_FLASH);
        const float fl = v.f(P_FLICKER);
        if (fl > 0.f && stops > 0.f) {
            uint64_t hsh = mix64(mix64(uint64_t(v.v[P_SEED][0])) ^ uint64_t(int64_t(frame) + (int64_t(1) << 40)));
            float r = float(hsh >> 40) / 16777216.f * 2.f - 1.f;
            stops = std::max(0.f, stops + fl * r * std::min(1.f, stops / 2.f));
        }
        pl.stops = stops;
        pl.blur = flash::blurFrames(sp, stops);
    }
    return pl;
}

bool fetch(OfxImageClipHandle clip, double t, flash::Image& img, OfxRectI& bounds) {
    OfxPropertySetHandle h = nullptr;
    if (!clip || gFx->clipGetImage(clip, t, nullptr, &h) != kOfxStatOK || !h) return false;
    void* data = nullptr;
    int rowBytes = 0;
    char* depth = nullptr;
    char* comps = nullptr;
    gProp->propGetPointer(h, kOfxImagePropData, 0, &data);
    gProp->propGetInt(h, kOfxImagePropRowBytes, 0, &rowBytes);
    gProp->propGetIntN(h, kOfxImagePropBounds, 4, &bounds.x1);
    gProp->propGetString(h, kOfxImageEffectPropPixelDepth, 0, &depth);
    gProp->propGetString(h, kOfxImageEffectPropComponents, 0, &comps);
    const int w = bounds.x2 - bounds.x1, hh = bounds.y2 - bounds.y1;
    bool ok = data && w > 0 && hh > 0 && depth && comps &&
              !std::strcmp(depth, kOfxBitDepthFloat) && !std::strcmp(comps, kOfxImageComponentRGBA);
    if (ok) {
        img = flash::Image(w, hh);
        for (int y = 0; y < hh; ++y)
            std::memcpy(img.row(y), static_cast<const char*>(data) + std::ptrdiff_t(y) * rowBytes,
                        size_t(w) * 4 * sizeof(float));
    }
    gFx->clipReleaseImage(h);
    return ok;
}

// ---------------------------------------------------------------- actions

OfxStatus onLoad() {
    if (!gHost) return kOfxStatErrMissingHostFeature;
    gProp = static_cast<const OfxPropertySuiteV1*>(gHost->fetchSuite(gHost->host, kOfxPropertySuite, 1));
    gFx = static_cast<const OfxImageEffectSuiteV1*>(gHost->fetchSuite(gHost->host, kOfxImageEffectSuite, 1));
    gParam = static_cast<const OfxParameterSuiteV1*>(gHost->fetchSuite(gHost->host, kOfxParameterSuite, 1));
    return (gProp && gFx && gParam) ? kOfxStatOK : kOfxStatErrMissingHostFeature;
}

OfxStatus describe(OfxImageEffectHandle effect) {
    OfxPropertySetHandle p;
    gFx->getPropertySet(effect, &p);
    gProp->propSetString(p, kOfxPropLabel, 0, "FilmFlash");
    gProp->propSetString(p, kOfxImageEffectPluginPropGrouping, 0, kGrouping);
    gProp->propSetString(p, kOfxPropPluginDescription, 0,
                         "The exposure flash of a film camera running down and back up to "
                         "speed, with the soft shoulder, fog and halation of an overexposed "
                         "negative. Use as a transition on a cut, or as an effect.");
    gProp->propSetString(p, kOfxImageEffectPropSupportedContexts, 0, kOfxImageEffectContextTransition);
    gProp->propSetString(p, kOfxImageEffectPropSupportedContexts, 1, kOfxImageEffectContextFilter);
    gProp->propSetString(p, kOfxImageEffectPropSupportedContexts, 2, kOfxImageEffectContextGeneral);
    gProp->propSetString(p, kOfxImageEffectPropSupportedPixelDepths, 0, kOfxBitDepthFloat);
    gProp->propSetInt(p, kOfxImageEffectPropSupportsTiles, 0, 0);
    gProp->propSetInt(p, kOfxImageEffectPropTemporalClipAccess, 0, 1);
    gProp->propSetInt(p, kOfxImageEffectPropSupportsMultiResolution, 0, 1);
    gProp->propSetInt(p, kOfxImageEffectPropSupportsMultipleClipDepths, 0, 0);
    gProp->propSetInt(p, kOfxImageEffectPluginPropHostFrameThreading, 0, 0);
    gProp->propSetString(p, kOfxImageEffectPluginRenderThreadSafety, 0, kOfxImageEffectRenderFullySafe);
    return kOfxStatOK;
}

void defineClip(OfxImageEffectHandle effect, const char* name) {
    OfxPropertySetHandle c;
    gFx->clipDefine(effect, name, &c);
    gProp->propSetString(c, kOfxImageEffectPropSupportedComponents, 0, kOfxImageComponentRGBA);
    gProp->propSetInt(c, kOfxImageEffectPropSupportsTiles, 0, 0);
    if (std::strcmp(name, kOfxImageEffectOutputClipName) != 0)
        gProp->propSetInt(c, kOfxImageEffectPropTemporalClipAccess, 0, 1);
}

OfxStatus describeInContext(OfxImageEffectHandle effect, OfxPropertySetHandle inArgs) {
    char* context = nullptr;
    gProp->propGetString(inArgs, kOfxImageEffectPropContext, 0, &context);
    const bool transition = context && !std::strcmp(context, kOfxImageEffectContextTransition);
    if (transition) {
        defineClip(effect, kOfxImageEffectTransitionSourceFromClipName);
        defineClip(effect, kOfxImageEffectTransitionSourceToClipName);
    } else {
        defineClip(effect, kOfxImageEffectSimpleSourceClipName);
    }
    defineClip(effect, kOfxImageEffectOutputClipName);

    OfxParamSetHandle ps;
    gFx->getParamSet(effect, &ps);
    OfxPropertySetHandle p;
    if (transition) {
        // Mandated by OFX; the host animates it from 0 to 1 across the transition.
        gParam->paramDefine(ps, kOfxParamTypeDouble, kOfxImageEffectTransitionParamName, &p);
        gProp->propSetString(p, kOfxPropLabel, 0, "Transition");
        gProp->propSetDouble(p, kOfxParamPropDefault, 0, 0.0);
        gProp->propSetDouble(p, kOfxParamPropMin, 0, 0.0);
        gProp->propSetDouble(p, kOfxParamPropMax, 0, 1.0);
        gProp->propSetDouble(p, kOfxParamPropDisplayMin, 0, 0.0);
        gProp->propSetDouble(p, kOfxParamPropDisplayMax, 0, 1.0);
    }

    const char* openGroup = nullptr;
    for (const PDef& d : defs()) {
        if (d.ctx == kFilterOnly && transition) continue;
        if (d.ctx == kTransitionOnly && !transition) continue;
        if (d.group && (!openGroup || std::strcmp(openGroup, d.group))) {
            for (const GDef& g : kGroups)
                if (!std::strcmp(g.name, d.group)) {
                    OfxPropertySetHandle gp;
                    gParam->paramDefine(ps, kOfxParamTypeGroup, g.name, &gp);
                    gProp->propSetString(gp, kOfxPropLabel, 0, g.label);
                    gProp->propSetInt(gp, kOfxParamPropGroupOpen, 0, g.open ? 1 : 0);
                }
            openGroup = d.group;
        }
        static const char* const types[] = {kOfxParamTypeDouble, kOfxParamTypeInteger,
                                            kOfxParamTypeChoice, kOfxParamTypeRGB};
        gParam->paramDefine(ps, types[d.type], d.name, &p);
        gProp->propSetString(p, kOfxPropLabel, 0, d.label);
        gProp->propSetString(p, kOfxParamPropHint, 0, d.hint);
        if (d.group) gProp->propSetString(p, kOfxParamPropParent, 0, d.group);
        switch (d.type) {
            case kDbl:
                gProp->propSetDouble(p, kOfxParamPropDefault, 0, d.def[0]);
                gProp->propSetDouble(p, kOfxParamPropMin, 0, d.lo);
                gProp->propSetDouble(p, kOfxParamPropMax, 0, d.hi);
                gProp->propSetDouble(p, kOfxParamPropDisplayMin, 0, d.dlo);
                gProp->propSetDouble(p, kOfxParamPropDisplayMax, 0, d.dhi);
                gProp->propSetDouble(p, kOfxParamPropIncrement, 0, 0.01);
                gProp->propSetInt(p, kOfxParamPropDigits, 0, 2);
                break;
            case kInt:
                gProp->propSetInt(p, kOfxParamPropDefault, 0, int(d.def[0]));
                gProp->propSetInt(p, kOfxParamPropMin, 0, int(d.lo));
                gProp->propSetInt(p, kOfxParamPropMax, 0, int(d.hi));
                gProp->propSetInt(p, kOfxParamPropDisplayMin, 0, int(d.dlo));
                gProp->propSetInt(p, kOfxParamPropDisplayMax, 0, int(d.dhi));
                gProp->propSetInt(p, kOfxParamPropAnimates, 0, 0);
                break;
            case kChoice:
                for (size_t i = 0; i < d.options.size(); ++i)
                    gProp->propSetString(p, kOfxParamPropChoiceOption, int(i), d.options[i]);
                gProp->propSetInt(p, kOfxParamPropDefault, 0, int(d.def[0]));
                gProp->propSetInt(p, kOfxParamPropAnimates, 0, 0);
                break;
            case kRGB:
                gProp->propSetDoubleN(p, kOfxParamPropDefault, 3, d.def);
                break;
        }
    }
    return kOfxStatOK;
}

OfxStatus createInstance(OfxImageEffectHandle effect) {
    auto* in = new Instance;
    OfxPropertySetHandle props;
    gFx->getPropertySet(effect, &props);
    char* context = nullptr;
    gProp->propGetString(props, kOfxImageEffectPropContext, 0, &context);
    in->transition = context && !std::strcmp(context, kOfxImageEffectContextTransition);
    if (in->transition) {
        gFx->clipGetHandle(effect, kOfxImageEffectTransitionSourceFromClipName, &in->from, nullptr);
        gFx->clipGetHandle(effect, kOfxImageEffectTransitionSourceToClipName, &in->to, nullptr);
    } else {
        gFx->clipGetHandle(effect, kOfxImageEffectSimpleSourceClipName, &in->src, nullptr);
    }
    gFx->clipGetHandle(effect, kOfxImageEffectOutputClipName, &in->dst, nullptr);
    OfxParamSetHandle ps;
    gFx->getParamSet(effect, &ps);
    for (const PDef& d : defs())
        if (gParam->paramGetHandle(ps, d.name, &in->h[d.id], nullptr) != kOfxStatOK) in->h[d.id] = nullptr;
    if (in->transition &&
        gParam->paramGetHandle(ps, kOfxImageEffectTransitionParamName, &in->transitionParam, nullptr) != kOfxStatOK)
        in->transitionParam = nullptr;
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

OfxStatus isIdentity(OfxImageEffectHandle effect, OfxPropertySetHandle inArgs,
                     OfxPropertySetHandle outArgs) {
    Instance* in = getInstance(effect);
    double t = 0;
    gProp->propGetDouble(inArgs, kOfxPropTime, 0, &t);
    Plan pl = planFor(in, readValues(in, t), t);
    if (!pl.identity(t)) return kOfxStatReplyDefault;
    const char* clip = !in->transition ? kOfxImageEffectSimpleSourceClipName
                       : pl.toWeight >= 1.f ? kOfxImageEffectTransitionSourceToClipName
                                            : kOfxImageEffectTransitionSourceFromClipName;
    gProp->propSetString(outArgs, kOfxPropName, 0, clip);
    gProp->propSetDouble(outArgs, kOfxPropTime, 0, t);
    return kOfxStatOK;
}

OfxStatus getFramesNeeded(OfxImageEffectHandle effect, OfxPropertySetHandle inArgs,
                          OfxPropertySetHandle outArgs) {
    Instance* in = getInstance(effect);
    double t = 0;
    gProp->propGetDouble(inArgs, kOfxPropTime, 0, &t);
    const Plan pl = planFor(in, readValues(in, t), t);
    auto range = [&](const char* clip, double s) {
        double r[2] = {std::floor(s - pl.blur / 2.0 - 0.5), std::ceil(s + pl.blur / 2.0 + 0.5)};
        std::string prop = std::string("OfxImageClipPropFrameRange_") + clip;
        gProp->propSetDoubleN(outArgs, prop.c_str(), 2, r);
    };
    if (in->transition) {
        range(kOfxImageEffectTransitionSourceFromClipName, pl.sFrom);
        range(kOfxImageEffectTransitionSourceToClipName, pl.sTo);
    } else {
        range(kOfxImageEffectSimpleSourceClipName, pl.sFrom);
    }
    return kOfxStatOK;
}

// The picture from `clip` at source time `s`, averaged over a `width`-frame
// exposure. Falls back to the frame at `t` if the host can't supply them.
bool gather(OfxImageClipHandle clip, double s, float width, double t, int encoding,
            flash::Image& out, OfxRectI& bounds) {
    std::vector<flash::Image> imgs;
    std::vector<float> wts;
    for (auto& tap : flash::blurTaps(s, width)) {
        flash::Image im;
        OfxRectI r;
        if (!fetch(clip, tap.first, im, r)) continue;
        if (!imgs.empty() && (im.w != imgs[0].w || im.h != imgs[0].h)) continue;
        if (imgs.empty()) bounds = r;
        imgs.push_back(std::move(im));
        wts.push_back(tap.second);
    }
    if (imgs.empty()) return fetch(clip, t, out, bounds);
    float total = 0;
    for (float w : wts) total += w;
    std::vector<const flash::Image*> ptrs;
    for (size_t i = 0; i < imgs.size(); ++i) {
        ptrs.push_back(&imgs[i]);
        wts[i] /= total;
    }
    flash::blendFrames(ptrs, wts, encoding, out);
    return true;
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
    const Values v = readValues(in, t);
    const Plan pl = planFor(in, v, t);
    flash::Look lk = lookFrom(v);

    flash::Image a, b;
    OfxRectI ab{0, 0, 0, 0}, bb{0, 0, 0, 0};
    bool haveA = false, haveB = false;
    if (!in->transition) {
        haveA = gather(in->src, pl.sFrom, pl.blur, t, lk.encoding, a, ab);
    } else {
        if (pl.toWeight < 1.f) haveA = gather(in->from, pl.sFrom, pl.blur, t, lk.encoding, a, ab);
        if (pl.toWeight > 0.f) haveB = gather(in->to, pl.sTo, pl.blur, t, lk.encoding, b, bb);
    }
    if (!haveA && !haveB) return gFx->abort(effect) ? kOfxStatOK : kOfxStatFailed;

    const int width = haveA ? a.w : b.w;
    lk.halationRadius = v.f(P_HALSIZE) / 100.f * float(width);
    if (haveA) flash::render(a, pl.stops, lk, a);
    if (haveB) flash::render(b, pl.stops, lk, b);
    const float wb = haveA && haveB ? pl.toWeight : (haveB ? 1.f : 0.f);

    OfxPropertySetHandle dh = nullptr;
    if (gFx->clipGetImage(in->dst, t, nullptr, &dh) != kOfxStatOK || !dh) return kOfxStatFailed;
    void* dd = nullptr;
    int drb = 0;
    OfxRectI db;
    gProp->propGetPointer(dh, kOfxImagePropData, 0, &dd);
    gProp->propGetInt(dh, kOfxImagePropRowBytes, 0, &drb);
    gProp->propGetIntN(dh, kOfxImagePropBounds, 4, &db.x1);
    const int x1 = std::max(win.x1, db.x1), x2 = std::min(win.x2, db.x2);
    const int y1 = std::max(win.y1, db.y1), y2 = std::min(win.y2, db.y2);
    auto sample = [](const flash::Image& img, const OfxRectI& r, int x, int y) -> const float* {
        int sx = x - r.x1, sy = y - r.y1;
        if (img.empty() || sx < 0 || sy < 0 || sx >= img.w || sy >= img.h) return nullptr;
        return img.row(sy) + size_t(sx) * 4;
    };
    flash::parallelFor(std::max(0, y2 - y1), [&](int j) {
        const int y = y1 + j;
        float* d = reinterpret_cast<float*>(static_cast<char*>(dd) + std::ptrdiff_t(y - db.y1) * drb) +
                   size_t(x1 - db.x1) * 4;
        for (int x = x1; x < x2; ++x, d += 4) {
            const float* pa = haveA ? sample(a, ab, x, y) : nullptr;
            const float* pb = haveB ? sample(b, bb, x, y) : nullptr;
            for (int k = 0; k < 4; ++k) {
                float va = pa ? pa[k] : 0.f, vb = pb ? pb[k] : 0.f;
                d[k] = va + (vb - va) * wb;
            }
        }
    });
    gFx->clipReleaseImage(dh);
    return kOfxStatOK;
}

OfxStatus mainEntry(const char* action, const void* handle, OfxPropertySetHandle inArgs,
                    OfxPropertySetHandle outArgs) {
    try {
        auto effect = (OfxImageEffectHandle)handle;
        if (!std::strcmp(action, kOfxActionLoad)) return onLoad();
        if (!std::strcmp(action, kOfxActionUnload)) return kOfxStatOK;
        if (!std::strcmp(action, kOfxActionDescribe)) return describe(effect);
        if (!std::strcmp(action, kOfxImageEffectActionDescribeInContext))
            return describeInContext(effect, inArgs);
        if (!std::strcmp(action, kOfxActionCreateInstance)) return createInstance(effect);
        if (!std::strcmp(action, kOfxActionDestroyInstance)) return destroyInstance(effect);
        if (!std::strcmp(action, kOfxImageEffectActionIsIdentity))
            return isIdentity(effect, inArgs, outArgs);
        if (!std::strcmp(action, kOfxImageEffectActionGetFramesNeeded))
            return getFramesNeeded(effect, inArgs, outArgs);
        if (!std::strcmp(action, kOfxImageEffectActionRender)) return render(effect, inArgs);
        return kOfxStatReplyDefault;
    } catch (const std::bad_alloc&) {
        return kOfxStatErrMemory;
    } catch (...) {
        return kOfxStatFailed;
    }
}

void setHost(OfxHost* host) { gHost = host; }

OfxPlugin gPlugin = {kOfxImageEffectPluginApi, 1, kPluginId, 1, 1, setHost, mainEntry};

}  // namespace

FF_EXPORT int OfxGetNumberOfPlugins(void) { return 1; }
FF_EXPORT OfxPlugin* OfxGetPlugin(int nth) { return nth == 0 ? &gPlugin : nullptr; }
