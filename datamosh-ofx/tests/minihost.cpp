// minihost: a tiny OFX host for testing the DataMosh plugin without Resolve.
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
    double value = 0;
};
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
static OfxStatus getValueV(OfxParamHandle h, va_list ap) {
    auto* p = reinterpret_cast<Param*>(h);
    if (!p) return kOfxStatErrBadHandle;
    if (p->type == kOfxParamTypePushButton) return kOfxStatErrUnsupported;
    if (isDouble(p)) *va_arg(ap, double*) = p->value;
    else *va_arg(ap, int*) = int(p->value);
    return kOfxStatOK;
}
static OfxStatus paramGetValue(OfxParamHandle h, ...) {
    va_list ap; va_start(ap, h); OfxStatus s = getValueV(h, ap); va_end(ap); return s;
}
static OfxStatus paramGetValueAtTime(OfxParamHandle h, OfxTime t, ...) {
    va_list ap; va_start(ap, t); OfxStatus s = getValueV(h, ap); va_end(ap); return s;
}
static OfxStatus setValueV(OfxParamHandle h, va_list ap) {
    auto* p = reinterpret_cast<Param*>(h);
    if (isDouble(p)) p->value = va_arg(ap, double);
    else p->value = va_arg(ap, int);
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
static Video gVideo;

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
    int f = int(std::lround(t));
    if (f < 0 || f >= gVideo.n) return kOfxStatFailed;
    auto* img = new Image;
    img->data.resize(size_t(w) * h * 4);
    // OFX images are bottom-up; store rows flipped like a real host would.
    for (int y = 0; y < h; ++y) {
        const unsigned char* s = &gVideo.rgb[(size_t(f) * h + (h - 1 - y)) * w * 3];
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
    setS(reinterpret_cast<OfxPropertySetHandle>(&fx->props), kOfxImageEffectPropContext, 0, kOfxImageEffectContextFilter);
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
    if (act(kOfxImageEffectActionGetFramesNeeded, fx, &in, &fout) != kOfxStatOK) die("getFramesNeeded");
    double range[2];
    if (getDN(reinterpret_cast<OfxPropertySetHandle>(&fout), "OfxImageClipPropFrameRange_Source", 2, range))
        die("getFramesNeeded did not set a range");

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
        std::fprintf(stderr, "usage: minihost PLUGIN IN.rgb W H OUT.rgb [--set name=value] [--start-here N]\n");
        return 2;
    }
    gVideo.w = std::atoi(argv[3]);
    gVideo.h = std::atoi(argv[4]);
    std::vector<std::pair<std::string, double>> sets;
    int startHere = -1, applyLook = -1;
    std::vector<std::pair<std::string, double>> expects;
    for (int i = 6; i < argc; ++i) {
        std::string a = argv[i];
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
    if (std::string(argv[2]) == "synthetic") {
        // Shot A (0-23): still rings. Shot B (24-47): stripes and a square
        // panning diagonally, so there is real motion to mosh with.
        gVideo.n = 48;
        gVideo.rgb.resize(fb * gVideo.n);
        for (int t = 0; t < gVideo.n; ++t)
            for (int y = 0; y < gVideo.h; ++y)
                for (int x = 0; x < gVideo.w; ++x) {
                    unsigned char* p = &gVideo.rgb[((size_t(t) * gVideo.h + y) * gVideo.w + x) * 3];
                    if (t < 24) {
                        double r = std::hypot(x - gVideo.w / 2.0, y - gVideo.h / 2.0);
                        p[0] = (unsigned char)(127 + 120 * std::sin(r * 0.3));
                        p[1] = (unsigned char)(127 + 120 * std::cos(r * 0.2));
                        p[2] = 200;
                    } else {
                        int u = x + 3 * (t - 24), v = y + 2 * (t - 24);
                        bool sq = (u / 24 + v / 24) % 2 == 0;
                        p[0] = (unsigned char)(sq ? 230 : 30);
                        p[1] = (unsigned char)((u * 4) & 255);
                        p[2] = (unsigned char)((v * 5) & 255);
                    }
                }
    } else {
    FILE* f = std::fopen(argv[2], "rb");
    if (!f) die("open input");
    std::fseek(f, 0, SEEK_END);
    long sz = std::ftell(f);
    std::fseek(f, 0, SEEK_SET);
    gVideo.n = int(sz / long(fb));
    gVideo.rgb.resize(fb * gVideo.n);
    if (std::fread(gVideo.rgb.data(), 1, gVideo.rgb.size(), f) != gVideo.rgb.size()) die("read input");
    std::fclose(f);
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
    PropSet ctx;
    setS(reinterpret_cast<OfxPropertySetHandle>(&ctx), kOfxImageEffectPropContext, 0, kOfxImageEffectContextFilter);
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
    std::vector<std::vector<float>> seq(gVideo.n);
    gSourceFetches = 0;
    for (int t = 0; t < gVideo.n; ++t) seq[t] = renderFrame(fx.get(), t);
    std::printf("sequential: %d frames, %d source fetches (%.2f per frame)\n", gVideo.n,
                gSourceFetches, double(gSourceFetches) / gVideo.n);

    // Pass 2: fresh instance, shuffled order (like scrubbing); must match.
    auto fx2 = makeInstance(descCtx, sets);
    std::vector<int> order(gVideo.n);
    for (int i = 0; i < gVideo.n; ++i) order[i] = i;
    std::shuffle(order.begin(), order.end(), std::mt19937(1234));
    double worst = 0;
    for (int t : order) {
        std::vector<float> r = renderFrame(fx2.get(), t);
        for (size_t i = 0; i < r.size(); ++i) worst = std::max(worst, double(std::fabs(r[i] - seq[t][i])));
    }
    std::printf("shuffled render max difference vs sequential: %g\n", worst);
    if (worst > 1e-5) die("shuffled render differs from sequential render");

    act(kOfxActionDestroyInstance, fx.get());
    act(kOfxActionDestroyInstance, fx2.get());
    act(kOfxActionUnload, nullptr);
    if (gLiveImages != 0) die("plugin leaked source images (missing clipReleaseImage)");

    FILE* o = std::fopen(argv[5], "wb");
    std::vector<unsigned char> buf(fb);
    const int w = gVideo.w, h = gVideo.h;
    for (int t = 0; t < gVideo.n; ++t) {
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
