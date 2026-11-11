// minihost: a tiny OFX host for testing the EriksTV plugin without Resolve.
//
// It dlopens the plugin binary, runs the same actions a host runs (load,
// describe, create instance, isIdentity, getFramesNeeded, render), feeds it
// frames from a raw rgb24 file and writes the rendered frames back out as raw
// rgb24. It also renders the frames again in shuffled order on a fresh
// instance and checks the result is identical, which exercises the caches.
//
//   minihost PLUGIN.so IN.rgb W H OUT.rgb [--set name=value ...] [--start-here N]
#include <dlfcn.h>

#include <algorithm>
#include <cmath>
#include <cstdarg>
#include <cstdio>
#include <cstdlib>
#include <cstring>
#include <map>
#include <memory>
#include <random>
#include <string>
#include <vector>

#include "ofxCore.h"
#include "ofxImageEffect.h"
#include "ofxMessage.h"
#include "ofxMultiThread.h"
#include "ofxParam.h"
#include "ofxProperty.h"

// ------------------------------------------------------------ properties

struct Prop {
    std::vector<std::string> s;
    std::vector<double> d;
    std::vector<int> i;
    std::vector<void*> p;
};
struct PropSet {
    std::map<std::string, Prop> m;
};
#define PS(h) (reinterpret_cast<PropSet*>(h))

template <class V, class T>
static void put(V& v, int idx, T x) {
    if (int(v.size()) <= idx) v.resize(idx + 1);
    v[idx] = x;
}

static OfxStatus setP(OfxPropertySetHandle h, const char* n, int i, void* v) { put(PS(h)->m[n].p, i, v); return kOfxStatOK; }
static OfxStatus setS(OfxPropertySetHandle h, const char* n, int i, const char* v) { put(PS(h)->m[n].s, i, std::string(v ? v : "")); return kOfxStatOK; }
static OfxStatus setD(OfxPropertySetHandle h, const char* n, int i, double v) { put(PS(h)->m[n].d, i, v); return kOfxStatOK; }
static OfxStatus setI(OfxPropertySetHandle h, const char* n, int i, int v) { put(PS(h)->m[n].i, i, v); return kOfxStatOK; }
static OfxStatus setPN(OfxPropertySetHandle h, const char* n, int c, void* const* v) { for (int i = 0; i < c; ++i) setP(h, n, i, v[i]); return kOfxStatOK; }
static OfxStatus setSN(OfxPropertySetHandle h, const char* n, int c, const char* const* v) { for (int i = 0; i < c; ++i) setS(h, n, i, v[i]); return kOfxStatOK; }
static OfxStatus setDN(OfxPropertySetHandle h, const char* n, int c, const double* v) { for (int i = 0; i < c; ++i) setD(h, n, i, v[i]); return kOfxStatOK; }
static OfxStatus setIN(OfxPropertySetHandle h, const char* n, int c, const int* v) { for (int i = 0; i < c; ++i) setI(h, n, i, v[i]); return kOfxStatOK; }

static Prop* find(OfxPropertySetHandle h, const char* n) {
    if (!h) return nullptr;
    auto it = PS(h)->m.find(n);
    return it == PS(h)->m.end() ? nullptr : &it->second;
}
static OfxStatus getP(OfxPropertySetHandle h, const char* n, int i, void** v) {
    Prop* p = find(h, n);
    if (!p || int(p->p.size()) <= i) return kOfxStatErrUnknown;
    *v = p->p[i];
    return kOfxStatOK;
}
static OfxStatus getS(OfxPropertySetHandle h, const char* n, int i, char** v) {
    Prop* p = find(h, n);
    if (!p || int(p->s.size()) <= i) return kOfxStatErrUnknown;
    *v = const_cast<char*>(p->s[i].c_str());
    return kOfxStatOK;
}
static OfxStatus getD(OfxPropertySetHandle h, const char* n, int i, double* v) {
    Prop* p = find(h, n);
    if (!p || int(p->d.size()) <= i) return kOfxStatErrUnknown;
    *v = p->d[i];
    return kOfxStatOK;
}
static OfxStatus getI(OfxPropertySetHandle h, const char* n, int i, int* v) {
    Prop* p = find(h, n);
    if (!p || int(p->i.size()) <= i) return kOfxStatErrUnknown;
    *v = p->i[i];
    return kOfxStatOK;
}
static OfxStatus getPN(OfxPropertySetHandle h, const char* n, int c, void** v) { for (int i = 0; i < c; ++i) if (getP(h, n, i, v + i)) return kOfxStatErrUnknown; return kOfxStatOK; }
static OfxStatus getSN(OfxPropertySetHandle h, const char* n, int c, char** v) { for (int i = 0; i < c; ++i) if (getS(h, n, i, v + i)) return kOfxStatErrUnknown; return kOfxStatOK; }
static OfxStatus getDN(OfxPropertySetHandle h, const char* n, int c, double* v) { for (int i = 0; i < c; ++i) if (getD(h, n, i, v + i)) return kOfxStatErrUnknown; return kOfxStatOK; }
static OfxStatus getIN(OfxPropertySetHandle h, const char* n, int c, int* v) { for (int i = 0; i < c; ++i) if (getI(h, n, i, v + i)) return kOfxStatErrUnknown; return kOfxStatOK; }
static OfxStatus reset(OfxPropertySetHandle, const char*) { return kOfxStatOK; }
static OfxStatus dim(OfxPropertySetHandle h, const char* n, int* c) {
    Prop* p = find(h, n);
    if (!p) return kOfxStatErrUnknown;
    *c = int(std::max({p->s.size(), p->d.size(), p->i.size(), p->p.size()}));
    return kOfxStatOK;
}

static OfxPropertySuiteV1 gPropSuite = {setP, setS, setD, setI, setPN, setSN, setDN, setIN,
                                        getP, getS, getD, getI, getPN, getSN, getDN, getIN,
                                        reset, dim};

// ------------------------------------------------------------ params

struct Param {
    std::string type;
    PropSet props;
    double value = 0;       // the value, or the first component
    double value2 = 0, value3 = 0;  // RGB's other components
    bool transitionCurve = false;   // the host animates it: value = t / (frames - 1)
};
static int gFrames = 1;   // frames rendered (the transition's length)
static int gOffset = 0;   // clip time of the first rendered frame
struct ParamSet {
    PropSet props;
    std::map<std::string, std::unique_ptr<Param>> params;
    std::vector<std::string> order;
};

static OfxStatus paramDefine(OfxParamSetHandle ps, const char* type, const char* name, OfxPropertySetHandle* props) {
    auto* set = reinterpret_cast<ParamSet*>(ps);
    if (set->params.count(name)) return kOfxStatErrExists;
    auto p = std::make_unique<Param>();
    p->type = type;
    if (props) *props = reinterpret_cast<OfxPropertySetHandle>(&p->props);
    set->order.push_back(name);
    set->params[name] = std::move(p);
    return kOfxStatOK;
}
static OfxStatus paramGetHandle(OfxParamSetHandle ps, const char* name, OfxParamHandle* h, OfxPropertySetHandle* props) {
    auto* set = reinterpret_cast<ParamSet*>(ps);
    auto it = set->params.find(name);
    if (it == set->params.end()) return kOfxStatErrUnknown;
    *h = reinterpret_cast<OfxParamHandle>(it->second.get());
    if (props) *props = reinterpret_cast<OfxPropertySetHandle>(&it->second->props);
    return kOfxStatOK;
}
static OfxStatus paramSetGetPropertySet(OfxParamSetHandle ps, OfxPropertySetHandle* props) {
    *props = reinterpret_cast<OfxPropertySetHandle>(&reinterpret_cast<ParamSet*>(ps)->props);
    return kOfxStatOK;
}
static OfxStatus paramGetPropertySet(OfxParamHandle h, OfxPropertySetHandle* props) {
    *props = reinterpret_cast<OfxPropertySetHandle>(&reinterpret_cast<Param*>(h)->props);
    return kOfxStatOK;
}
static bool isDouble(const Param* p) { return p->type == kOfxParamTypeDouble; }
static OfxStatus getValueV(OfxParamHandle h, va_list ap, const double* time = nullptr) {
    auto* p = reinterpret_cast<Param*>(h);
    if (time && p->transitionCurve) {
        *va_arg(ap, double*) = gFrames > 1 ? (*time - gOffset) / (gFrames - 1) : 0.0;
        return kOfxStatOK;
    }
    if (!p) return kOfxStatErrBadHandle;
    if (p->type == kOfxParamTypePushButton) return kOfxStatErrUnsupported;
    if (p->type == kOfxParamTypeGroup) return kOfxStatErrUnsupported;
    if (p->type == kOfxParamTypeRGB) {
        *va_arg(ap, double*) = p->value;
        *va_arg(ap, double*) = p->value2;
        *va_arg(ap, double*) = p->value3;
    } else if (isDouble(p)) {
        *va_arg(ap, double*) = p->value;
    } else {
        *va_arg(ap, int*) = int(p->value);
    }
    return kOfxStatOK;
}
static OfxStatus paramGetValue(OfxParamHandle h, ...) {
    va_list ap; va_start(ap, h); OfxStatus s = getValueV(h, ap); va_end(ap); return s;
}
static OfxStatus paramGetValueAtTime(OfxParamHandle h, OfxTime t, ...) {
    va_list ap; va_start(ap, t); OfxStatus s = getValueV(h, ap, &t); va_end(ap); return s;
}
static OfxStatus setValueV(OfxParamHandle h, va_list ap) {
    auto* p = reinterpret_cast<Param*>(h);
    if (p->type == kOfxParamTypeRGB) {
        p->value = va_arg(ap, double);
        p->value2 = va_arg(ap, double);
        p->value3 = va_arg(ap, double);
    } else if (isDouble(p)) {
        p->value = va_arg(ap, double);
    } else {
        p->value = va_arg(ap, int);
    }
    return kOfxStatOK;
}
static OfxStatus paramSetValue(OfxParamHandle h, ...) {
    va_list ap; va_start(ap, h); OfxStatus s = setValueV(h, ap); va_end(ap); return s;
}
static OfxStatus paramSetValueAtTime(OfxParamHandle h, OfxTime t, ...) {
    va_list ap; va_start(ap, t); OfxStatus s = setValueV(h, ap); va_end(ap); return s;
}
static OfxStatus unsupported() { return kOfxStatErrUnsupported; }

static OfxParameterSuiteV1 makeParamSuite() {
    OfxParameterSuiteV1 s;
    std::memset(&s, 0, sizeof s);
    s.paramDefine = paramDefine;
    s.paramGetHandle = paramGetHandle;
    s.paramSetGetPropertySet = paramSetGetPropertySet;
    s.paramGetPropertySet = paramGetPropertySet;
    s.paramGetValue = paramGetValue;
    s.paramGetValueAtTime = paramGetValueAtTime;
    s.paramSetValue = paramSetValue;
    s.paramSetValueAtTime = paramSetValueAtTime;
    s.paramGetDerivative = reinterpret_cast<decltype(s.paramGetDerivative)>(unsupported);
    s.paramGetIntegral = reinterpret_cast<decltype(s.paramGetIntegral)>(unsupported);
    s.paramEditBegin = [](OfxParamSetHandle, const char*) -> OfxStatus { return kOfxStatOK; };
    s.paramEditEnd = [](OfxParamSetHandle) -> OfxStatus { return kOfxStatOK; };
    return s;
}
static OfxParameterSuiteV1 gParamSuite = makeParamSuite();

// ------------------------------------------------------------ effect

struct Video {
    int w = 0, h = 0, n = 0;
    std::vector<unsigned char> rgb;
};
static Video gVideo;   // "Source", or "SourceFrom" in a transition
static Video gVideoB;  // "SourceTo" in a transition
static std::string gContext = kOfxImageEffectContextFilter;

struct Clip {
    std::string name;
    PropSet props;
};
struct Image {
    PropSet props;
    std::vector<float> data;
};
struct Effect {
    PropSet props;
    ParamSet params;
    std::map<std::string, std::unique_ptr<Clip>> clips;
    std::unique_ptr<Image> output;  // output image for the current render
};

static int gSourceFetches = 0;
static int gLiveImages = 0;

static OfxStatus getPropertySet(OfxImageEffectHandle e, OfxPropertySetHandle* p) {
    *p = reinterpret_cast<OfxPropertySetHandle>(&reinterpret_cast<Effect*>(e)->props);
    return kOfxStatOK;
}
static OfxStatus getParamSet(OfxImageEffectHandle e, OfxParamSetHandle* p) {
    *p = reinterpret_cast<OfxParamSetHandle>(&reinterpret_cast<Effect*>(e)->params);
    return kOfxStatOK;
}
static OfxStatus clipDefine(OfxImageEffectHandle e, const char* name, OfxPropertySetHandle* p) {
    auto* fx = reinterpret_cast<Effect*>(e);
    auto c = std::make_unique<Clip>();
    c->name = name;
    *p = reinterpret_cast<OfxPropertySetHandle>(&c->props);
    fx->clips[name] = std::move(c);
    return kOfxStatOK;
}
static OfxStatus clipGetHandle(OfxImageEffectHandle e, const char* name, OfxImageClipHandle* c, OfxPropertySetHandle* p) {
    auto* fx = reinterpret_cast<Effect*>(e);
    auto it = fx->clips.find(name);
    if (it == fx->clips.end()) return kOfxStatErrBadIndex;
    *c = reinterpret_cast<OfxImageClipHandle>(it->second.get());
    if (p) *p = reinterpret_cast<OfxPropertySetHandle>(&it->second->props);
    return kOfxStatOK;
}
static OfxStatus clipGetPropertySet(OfxImageClipHandle c, OfxPropertySetHandle* p) {
    *p = reinterpret_cast<OfxPropertySetHandle>(&reinterpret_cast<Clip*>(c)->props);
    return kOfxStatOK;
}
static void fillImageProps(Image* img, int w, int h) {
    OfxPropertySetHandle p = reinterpret_cast<OfxPropertySetHandle>(&img->props);
    int b[4] = {0, 0, w, h};
    setIN(p, kOfxImagePropBounds, 4, b);
    setIN(p, kOfxImagePropRegionOfDefinition, 4, b);
    setI(p, kOfxImagePropRowBytes, 0, w * 4 * int(sizeof(float)));
    setS(p, kOfxImageEffectPropPixelDepth, 0, kOfxBitDepthFloat);
    setS(p, kOfxImageEffectPropComponents, 0, kOfxImageComponentRGBA);
    setP(p, kOfxImagePropData, 0, img->data.data());
}
static Effect* gCurrentEffect = nullptr;
static OfxStatus clipGetImage(OfxImageClipHandle ch, OfxTime t, const OfxRectD*, OfxPropertySetHandle* out) {
    auto* c = reinterpret_cast<Clip*>(ch);
    const int w = gVideo.w, h = gVideo.h;
    if (c->name == kOfxImageEffectOutputClipName) {
        if (!gCurrentEffect || !gCurrentEffect->output) return kOfxStatFailed;
        *out = reinterpret_cast<OfxPropertySetHandle>(&gCurrentEffect->output->props);
        return kOfxStatOK;
    }
    const Video& vid = c->name == kOfxImageEffectTransitionSourceToClipName ? gVideoB : gVideo;
    int f = int(std::lround(t));
    if (f < 0 || f >= vid.n) return kOfxStatFailed;
    auto* img = new Image;
    img->data.resize(size_t(w) * h * 4);
    // OFX images are bottom-up; store rows flipped like a real host would.
    for (int y = 0; y < h; ++y) {
        const unsigned char* s = &vid.rgb[(size_t(f) * h + (h - 1 - y)) * w * 3];
        float* d = &img->data[size_t(y) * w * 4];
        for (int x = 0; x < w; ++x) {
            d[x * 4 + 0] = s[x * 3 + 0] / 255.f;
            d[x * 4 + 1] = s[x * 3 + 1] / 255.f;
            d[x * 4 + 2] = s[x * 3 + 2] / 255.f;
            d[x * 4 + 3] = 1.f;
        }
    }
    fillImageProps(img, w, h);
    ++gSourceFetches;
    ++gLiveImages;
    *out = reinterpret_cast<OfxPropertySetHandle>(&img->props);
    return kOfxStatOK;
}
static OfxStatus clipReleaseImage(OfxPropertySetHandle p) {
    if (gCurrentEffect && gCurrentEffect->output &&
        p == reinterpret_cast<OfxPropertySetHandle>(&gCurrentEffect->output->props))
        return kOfxStatOK;
    // props is the first member of Image
    delete reinterpret_cast<Image*>(reinterpret_cast<PropSet*>(p));
    --gLiveImages;
    return kOfxStatOK;
}
static OfxStatus clipGetRoD(OfxImageClipHandle, OfxTime, OfxRectD* r) {
    *r = OfxRectD{0, 0, double(gVideo.w), double(gVideo.h)};
    return kOfxStatOK;
}
static int abortFx(OfxImageEffectHandle) { return 0; }

static OfxImageEffectSuiteV1 makeFxSuite() {
    OfxImageEffectSuiteV1 s;
    std::memset(&s, 0, sizeof s);
    s.getPropertySet = getPropertySet;
    s.getParamSet = getParamSet;
    s.clipDefine = clipDefine;
    s.clipGetHandle = clipGetHandle;
    s.clipGetPropertySet = clipGetPropertySet;
    s.clipGetImage = clipGetImage;
    s.clipReleaseImage = clipReleaseImage;
    s.clipGetRegionOfDefinition = clipGetRoD;
    s.abort = abortFx;
    return s;
}
static OfxImageEffectSuiteV1 gFxSuite = makeFxSuite();

static OfxStatus message(void*, const char* type, const char*, const char* fmt, ...) {
    va_list ap;
    va_start(ap, fmt);
    std::fprintf(stderr, "[plugin %s] ", type);
    std::vfprintf(stderr, fmt, ap);
    std::fprintf(stderr, "\n");
    va_end(ap);
    return kOfxStatOK;
}
static OfxMessageSuiteV1 gMsgSuite = {message};

static PropSet gHostProps;
static const void* fetchSuite(OfxPropertySetHandle, const char* name, int version) {
    if (!std::strcmp(name, kOfxPropertySuite) && version == 1) return &gPropSuite;
    if (!std::strcmp(name, kOfxImageEffectSuite) && version == 1) return &gFxSuite;
    if (!std::strcmp(name, kOfxParameterSuite) && version == 1) return &gParamSuite;
    if (!std::strcmp(name, kOfxMessageSuite) && version == 1) return &gMsgSuite;
    return nullptr;
}
static OfxHost gHost = {reinterpret_cast<OfxPropertySetHandle>(&gHostProps), fetchSuite};

// ------------------------------------------------------------ driver

static OfxPlugin* gPlugin = nullptr;

static OfxStatus act(const char* a, Effect* fx, PropSet* in = nullptr, PropSet* out = nullptr) {
    gCurrentEffect = fx;
    return gPlugin->mainEntry(a, fx, reinterpret_cast<OfxPropertySetHandle>(in),
                              reinterpret_cast<OfxPropertySetHandle>(out));
}

static void die(const char* m) {
    std::fprintf(stderr, "FAIL: %s\n", m);
    std::exit(1);
}

// Instance gets the descriptor's params, with defaults applied.
static std::unique_ptr<Effect> makeInstance(Effect& desc, const std::vector<std::pair<std::string, double>>& sets) {
    auto fx = std::make_unique<Effect>();
    for (auto& name : desc.params.order) {
        Param& d = *desc.params.params[name];
        auto p = std::make_unique<Param>();
        p->type = d.type;
        p->props = d.props;
        auto& def = d.props.m[kOfxParamPropDefault];
        p->value = !def.d.empty() ? def.d[0] : (!def.i.empty() ? def.i[0] : 0);
        if (def.d.size() >= 3) { p->value2 = def.d[1]; p->value3 = def.d[2]; }
        fx->params.order.push_back(name);
        fx->params.params[name] = std::move(p);
    }
    for (auto& [n, v] : sets) {
        auto it = fx->params.params.find(n);
        if (it == fx->params.params.end()) die(("unknown param " + n).c_str());
        it->second->value = v;
    }
    for (auto& [n, c] : desc.clips) {
        auto cc = std::make_unique<Clip>();
        cc->name = n;
        cc->props = c->props;
        fx->clips[n] = std::move(cc);
    }
    setS(reinterpret_cast<OfxPropertySetHandle>(&fx->props), kOfxImageEffectPropContext, 0, gContext.c_str());
    if (act(kOfxActionCreateInstance, fx.get()) != kOfxStatOK) die("createInstance");
    return fx;
}

// Renders frame t, returns RGBA floats (bottom-up).
static std::vector<float> renderFrame(Effect* fx, int t) {
    const int w = gVideo.w, h = gVideo.h;
    PropSet in, out;
    auto inH = reinterpret_cast<OfxPropertySetHandle>(&in);
    setD(inH, kOfxPropTime, 0, t);
    double rs[2] = {1, 1};
    setDN(inH, kOfxImageEffectPropRenderScale, 2, rs);
    setS(inH, kOfxImageEffectPropFieldToRender, 0, kOfxImageFieldNone);
    int win[4] = {0, 0, w, h};
    setIN(inH, kOfxImageEffectPropRenderWindow, 4, win);
    // In a transition the host animates the Transition param from 0 to 1.
    auto tp = fx->params.params.find(kOfxImageEffectTransitionParamName);
    if (tp != fx->params.params.end()) {
        tp->second->transitionCurve = true;
        tp->second->value = gFrames > 1 ? double(t - gOffset) / (gFrames - 1) : 0.0;
    }

    OfxStatus st = act(kOfxImageEffectActionIsIdentity, fx, &in, &out);
    if (st == kOfxStatOK) {
        char* clip = nullptr;
        double it = 0;
        getS(reinterpret_cast<OfxPropertySetHandle>(&out), kOfxPropName, 0, &clip);
        getD(reinterpret_cast<OfxPropertySetHandle>(&out), kOfxPropTime, 0, &it);
        OfxPropertySetHandle img;
        Effect* saved = gCurrentEffect;
        gCurrentEffect = nullptr;
        if (clipGetImage(reinterpret_cast<OfxImageClipHandle>(fx->clips[clip].get()), it, nullptr, &img)) die("identity fetch");
        std::vector<float> r = reinterpret_cast<Image*>(reinterpret_cast<PropSet*>(img))->data;
        clipReleaseImage(img);
        gCurrentEffect = saved;
        return r;
    }
    if (st != kOfxStatReplyDefault) die("isIdentity returned an error");

    PropSet fout;
    OfxStatus fs = act(kOfxImageEffectActionGetFramesNeeded, fx, &in, &fout);
    if (fs != kOfxStatOK && fs != kOfxStatReplyDefault) die("getFramesNeeded");
    if (fs == kOfxStatOK) {
        // Every input clip must get a sane range that includes t.
        for (auto& [name, clip] : fx->clips) {
            if (name == kOfxImageEffectOutputClipName) continue;
            double range[2];
            if (getDN(reinterpret_cast<OfxPropertySetHandle>(&fout),
                      ("OfxImageClipPropFrameRange_" + name).c_str(), 2, range))
                die(("getFramesNeeded did not set a range for " + name).c_str());
            if (!(range[0] <= range[1])) die("getFramesNeeded range is inverted");
        }
    }

    fx->output = std::make_unique<Image>();
    fx->output->data.assign(size_t(w) * h * 4, -1.f);
    fillImageProps(fx->output.get(), w, h);
    if (act(kOfxImageEffectActionRender, fx, &in, nullptr) != kOfxStatOK) die("render");
    std::vector<float> r = std::move(fx->output->data);
    fx->output.reset();
    for (float v : r)
        if (!std::isfinite(v) || v < 0.f) die("render left unwritten or invalid pixels");
    return r;
}

int main(int argc, char** argv) {
    if (argc < 6) {
        std::fprintf(stderr,
                     "usage: minihost PLUGIN IN.rgb|A.rgb+B.rgb|synthetic W H OUT.rgb [options]\n"
                     "  --context filter|transition   --set name=value   --start-here N\n"
                     "  --apply-look N   --expect name=value   --expect-identity-at N:Source|SourceFrom|SourceTo\n"
                     "  --max-below V   --expect-mean-above N:V   --expect-mean-below N:V\n"
                     "  --expect-fetches-above F (source frames fetched per rendered frame)\n");
        return 2;
    }
    gVideo.w = std::atoi(argv[3]);
    gVideo.h = std::atoi(argv[4]);
    std::vector<std::pair<std::string, double>> sets;
    int startHere = -1, applyLook = -1;
    std::vector<std::pair<std::string, double>> expects;
    std::vector<std::pair<int, std::string>> identityAt;
    std::vector<std::pair<int, double>> meanAbove, meanBelow;
    double maxBelow = -1, fetchesAbove = -1;
    int framesOpt = -1;
    auto frameColon = [](const std::string& kv, int& frame) {
        auto c = kv.find(':');
        frame = std::atoi(kv.substr(0, c).c_str());
        return kv.substr(c + 1);
    };
    for (int i = 6; i < argc; ++i) {
        std::string a = argv[i];
        if (a == "--context" && i + 1 < argc) {
            std::string c = argv[++i];
            gContext = c == "transition" ? kOfxImageEffectContextTransition : kOfxImageEffectContextFilter;
            continue;
        }
        if (a == "--expect-identity-at" && i + 1 < argc) {
            int fr; std::string clip = frameColon(argv[++i], fr);
            identityAt.emplace_back(fr, clip);
            continue;
        }
        if ((a == "--expect-mean-above" || a == "--expect-mean-below") && i + 1 < argc) {
            int fr; double v = std::atof(frameColon(argv[++i], fr).c_str());
            (a == "--expect-mean-above" ? meanAbove : meanBelow).emplace_back(fr, v);
            continue;
        }
        if (a == "--max-below" && i + 1 < argc) { maxBelow = std::atof(argv[++i]); continue; }
        if (a == "--expect-fetches-above" && i + 1 < argc) { fetchesAbove = std::atof(argv[++i]); continue; }
        if (a == "--frames" && i + 1 < argc) { framesOpt = std::atoi(argv[++i]); continue; }
        if (a == "--offset" && i + 1 < argc) { gOffset = std::atoi(argv[++i]); continue; }
        if (a == "--set" && i + 1 < argc) {
            std::string kv = argv[++i];
            auto eq = kv.find('=');
            sets.emplace_back(kv.substr(0, eq), std::atof(kv.substr(eq + 1).c_str()));
        } else if (a == "--start-here" && i + 1 < argc) {
            startHere = std::atoi(argv[++i]);
        } else if (a == "--apply-look" && i + 1 < argc) {
            applyLook = std::atoi(argv[++i]);
        } else if (a == "--expect" && i + 1 < argc) {
            std::string kv = argv[++i];
            auto eq = kv.find('=');
            expects.emplace_back(kv.substr(0, eq), std::atof(kv.substr(eq + 1).c_str()));
        }
    }

    const size_t fb = size_t(gVideo.w) * gVideo.h * 3;
    const bool transition = gContext == kOfxImageEffectContextTransition;
    gVideoB.w = gVideo.w;
    gVideoB.h = gVideo.h;
    // Shot A: still rings. Shot B: stripes and a square panning diagonally,
    // so there is real motion. A filter gets A (frames 0-23) then B (24-47)
    // in one clip; a transition gets all-A as SourceFrom, all-B as SourceTo.
    auto synth = [&](Video& vid, int shotOverride) {
        vid.n = 48;
        vid.rgb.resize(fb * vid.n);
        for (int t = 0; t < vid.n; ++t)
            for (int y = 0; y < vid.h; ++y)
                for (int x = 0; x < vid.w; ++x) {
                    unsigned char* p = &vid.rgb[((size_t(t) * vid.h + y) * vid.w + x) * 3];
                    const int shot = shotOverride >= 0 ? shotOverride : (t < 24 ? 0 : 1);
                    if (shot == 0) {
                        double r = std::hypot(x - vid.w / 2.0 - 2.0 * t, y - vid.h / 2.0);
                        p[0] = (unsigned char)(127 + 120 * std::sin(r * 0.3));
                        p[1] = (unsigned char)(127 + 120 * std::cos(r * 0.2));
                        p[2] = 200;
                    } else {
                        const int tt = shotOverride >= 0 ? t : t - 24;
                        int u = x + 3 * tt, v = y + 2 * tt;
                        bool sq = (u / 24 + v / 24) % 2 == 0;
                        p[0] = (unsigned char)(sq ? 230 : 30);
                        p[1] = (unsigned char)((u * 4) & 255);
                        p[2] = (unsigned char)((v * 5) & 255);
                    }
                }
    };
    auto load = [&](Video& vid, const std::string& path) {
        FILE* f = std::fopen(path.c_str(), "rb");
        if (!f) die(("open input " + path).c_str());
        std::fseek(f, 0, SEEK_END);
        long sz = std::ftell(f);
        std::fseek(f, 0, SEEK_SET);
        vid.n = int(sz / long(fb));
        vid.rgb.resize(fb * vid.n);
        if (std::fread(vid.rgb.data(), 1, vid.rgb.size(), f) != vid.rgb.size()) die("read input");
        std::fclose(f);
    };
    const std::string input = argv[2];
    if (input == "synthetic") {
        synth(gVideo, transition ? 0 : -1);
        if (transition) synth(gVideoB, 1);
    } else if (auto plus = input.find('+'); plus != std::string::npos) {
        load(gVideo, input.substr(0, plus));
        load(gVideoB, input.substr(plus + 1));
    } else {
        load(gVideo, input);
        if (transition) gVideoB = gVideo;
    }

    // Render N frames starting at clip time gOffset (the clips may be longer:
    // that's a transition with handles).
    {
        int avail = transition ? std::min(gVideo.n, gVideoB.n) : gVideo.n;
        gFrames = framesOpt > 0 ? framesOpt : avail - gOffset;
        if (gOffset < 0 || gFrames <= 0 || gOffset + gFrames > avail) die("--frames/--offset outside the clips");
    }

    void* lib = dlopen(argv[1], RTLD_NOW | RTLD_LOCAL);
    if (!lib) die(dlerror());
    auto getN = reinterpret_cast<int (*)()>(dlsym(lib, "OfxGetNumberOfPlugins"));
    auto getP = reinterpret_cast<OfxPlugin* (*)(int)>(dlsym(lib, "OfxGetPlugin"));
    if (!getN || !getP || getN() != 1) die("plugin entry points");
    gPlugin = getP(0);
    std::printf("plugin %s v%u.%u api %s\n", gPlugin->pluginIdentifier, gPlugin->pluginVersionMajor,
                gPlugin->pluginVersionMinor, gPlugin->pluginApi);
    gPlugin->setHost(&gHost);
    if (act(kOfxActionLoad, nullptr) != kOfxStatOK) die("load");

    Effect desc;
    if (act(kOfxActionDescribe, &desc) != kOfxStatOK) die("describe");
    {
        int nctx = 0;
        dim(reinterpret_cast<OfxPropertySetHandle>(&desc.props), kOfxImageEffectPropSupportedContexts, &nctx);
        bool ok = false;
        for (int i = 0; i < nctx; ++i) {
            char* c = nullptr;
            getS(reinterpret_cast<OfxPropertySetHandle>(&desc.props), kOfxImageEffectPropSupportedContexts, i, &c);
            ok = ok || (c && gContext == c);
        }
        if (!ok) die(("plugin does not support context " + gContext).c_str());
    }
    PropSet ctx;
    setS(reinterpret_cast<OfxPropertySetHandle>(&ctx), kOfxImageEffectPropContext, 0, gContext.c_str());
    Effect descCtx;
    descCtx.props = desc.props;
    if (act(kOfxImageEffectActionDescribeInContext, &descCtx, &ctx) != kOfxStatOK) die("describeInContext");
    std::printf("params:");
    for (auto& n : descCtx.params.order) std::printf(" %s", n.c_str());
    std::printf("\n");

    auto fx = makeInstance(descCtx, sets);
    if (startHere >= 0) {
        PropSet ic;
        auto h = reinterpret_cast<OfxPropertySetHandle>(&ic);
        setS(h, kOfxPropType, 0, kOfxTypeParameter);
        setS(h, kOfxPropName, 0, "setStartHere");
        setS(h, kOfxPropChangeReason, 0, kOfxChangeUserEdited);
        setD(h, kOfxPropTime, 0, startHere);
        if (act(kOfxActionInstanceChanged, fx.get(), &ic) != kOfxStatOK) die("instanceChanged");
        if (int(fx->params.params["moshStart"]->value) != startHere) die("Start Mosh Here did not set moshStart");
        std::printf("Start Mosh Here -> moshStart = %d\n", startHere);
        sets.emplace_back("moshStart", startHere);
    }

    auto press = [&](const char* button) {
        PropSet ic;
        auto h = reinterpret_cast<OfxPropertySetHandle>(&ic);
        setS(h, kOfxPropType, 0, kOfxTypeParameter);
        setS(h, kOfxPropName, 0, button);
        setS(h, kOfxPropChangeReason, 0, kOfxChangeUserEdited);
        setD(h, kOfxPropTime, 0, 0);
        if (act(kOfxActionInstanceChanged, fx.get(), &ic) != kOfxStatOK) die("instanceChanged");
    };
    if (applyLook >= 0) {
        fx->params.params["look"]->value = applyLook;
        press("applyLook");
        // The shuffled-order instance gets the same values.
        for (auto& n : fx->params.order) sets.emplace_back(n, fx->params.params[n]->value);
        std::printf("Apply Look %d\n", applyLook);
    }
    for (auto& [n, v] : expects) {
        auto it = fx->params.params.find(n);
        if (it == fx->params.params.end()) die(("expect: unknown param " + n).c_str());
        if (std::fabs(it->second->value - v) > 1e-9) {
            std::fprintf(stderr, "%s = %g, expected %g\n", n.c_str(), it->second->value, v);
            die("parameter has the wrong value");
        }
    }

    // Pass 1: play forward.
    const int N = gFrames;
    std::vector<std::vector<float>> seq(N);
    gSourceFetches = 0;
    for (int i = 0; i < N; ++i) seq[i] = renderFrame(fx.get(), gOffset + i);
    std::printf("sequential: %d frames, %d source fetches (%.2f per frame)\n", N,
                gSourceFetches, double(gSourceFetches) / N);
    if (fetchesAbove >= 0 && !(double(gSourceFetches) / N > fetchesAbove))
        die("plugin fetched fewer source frames than expected (no temporal access?)");

    // Pass 2: fresh instance, shuffled order (like scrubbing); must match.
    auto fx2 = makeInstance(descCtx, sets);
    std::vector<int> order(N);
    for (int i = 0; i < N; ++i) order[i] = i;
    std::shuffle(order.begin(), order.end(), std::mt19937(1234));
    double worst = 0;
    for (int t : order) {
        std::vector<float> r = renderFrame(fx2.get(), gOffset + t);
        for (size_t i = 0; i < r.size(); ++i) worst = std::max(worst, double(std::fabs(r[i] - seq[t][i])));
    }
    std::printf("shuffled render max difference vs sequential: %g\n", worst);
    if (worst > 1e-5) die("shuffled render differs from sequential render");

    // Optional checks on the rendered frames.
    auto sourceFrame = [&](const std::string& clip, int t) {
        Clip c;
        c.name = clip;
        OfxPropertySetHandle img;
        Effect* saved = gCurrentEffect;
        gCurrentEffect = nullptr;
        if (clipGetImage(reinterpret_cast<OfxImageClipHandle>(&c), t, nullptr, &img)) die("source fetch");
        std::vector<float> r = reinterpret_cast<Image*>(reinterpret_cast<PropSet*>(img))->data;
        clipReleaseImage(img);
        gCurrentEffect = saved;
        return r;
    };
    auto meanLuma = [&](int t) {
        double acc = 0;
        const auto& f = seq[t];
        for (size_t i = 0; i < f.size(); i += 4) acc += 0.2126 * f[i] + 0.7152 * f[i + 1] + 0.0722 * f[i + 2];
        return acc / double(f.size() / 4);
    };
    for (auto& [t, clip] : identityAt) {
        if (t < 0 || t >= N) die("--expect-identity-at: frame out of range");
        std::vector<float> src = sourceFrame(clip, gOffset + t);
        double d = 0;
        for (size_t i = 0; i < src.size(); ++i) d = std::max(d, double(std::fabs(src[i] - seq[t][i])));
        std::printf("frame %d vs %s: max difference %g\n", t, clip.c_str(), d);
        if (d > 1e-6) die("frame should be the untouched source");
    }
    for (auto& [t, v] : meanAbove) {
        double m = meanLuma(t);
        std::printf("frame %d mean luma %.4f (want > %g)\n", t, m, v);
        if (!(m > v)) die("frame is not bright enough");
    }
    for (auto& [t, v] : meanBelow) {
        double m = meanLuma(t);
        std::printf("frame %d mean luma %.4f (want < %g)\n", t, m, v);
        if (!(m < v)) die("frame is too bright");
    }
    if (maxBelow >= 0) {
        double mx = 0;
        for (auto& f : seq)
            for (size_t i = 0; i < f.size(); i += 4)
                mx = std::max({mx, double(f[i]), double(f[i + 1]), double(f[i + 2])});
        std::printf("brightest output value %.6f (want < %g)\n", mx, maxBelow);
        if (!(mx < maxBelow)) die("output reaches the ceiling (clipping)");
    }

    act(kOfxActionDestroyInstance, fx.get());
    act(kOfxActionDestroyInstance, fx2.get());
    act(kOfxActionUnload, nullptr);
    if (gLiveImages != 0) die("plugin leaked source images (missing clipReleaseImage)");

    FILE* o = std::fopen(argv[5], "wb");
    std::vector<unsigned char> buf(fb);
    const int w = gVideo.w, h = gVideo.h;
    for (int t = 0; t < N; ++t) {
        for (int y = 0; y < h; ++y)
            for (int x = 0; x < w; ++x)
                for (int k = 0; k < 3; ++k) {
                    float v = seq[t][(size_t(h - 1 - y) * w + x) * 4 + k];
                    v = std::min(std::max(v, 0.f), 1.f);
                    buf[(size_t(y) * w + x) * 3 + k] = (unsigned char)(v * 255.f + 0.5f);
                }
        std::fwrite(buf.data(), 1, fb, o);
    }
    std::fclose(o);
    std::printf("OK\n");
    return 0;
}
