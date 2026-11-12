// GateFlare: an OpenFX effect for DaVinci Resolve that recreates film-gate
// edge flare: bright sources just outside the frame line flare in from the
// edge of the gate as they enter or leave the shot.
//
// Plain OFX C API; the image work is in GateEngine.
#include <algorithm>
#include <cmath>
#include <cstdint>
#include <cstring>
#include <vector>

#include "Encodings.h"
#include "GateEngine.h"
#include "ofxCore.h"
#include "ofxImageEffect.h"
#include "ofxParam.h"
#include "ofxProperty.h"

#if defined(_WIN32)
#define GF_EXPORT extern "C" __declspec(dllexport)
#else
#define GF_EXPORT extern "C" __attribute__((visibility("default")))
#endif

namespace {

const char* const kPluginId = "com.lumpy.GateFlare";

enum P {
    P_CROP, P_CROPOUT, P_SHOWGATE,
    P_SHAPE, P_BLADEANGLE, P_BLADESOFT, P_COLOURMODE, P_COMPOSITE,
    P_STREAKS, P_STREAKDETAIL, P_SHIMMER, P_SHIMMERSPEED, P_CORE, P_FRINGE, P_SEED,
    P_THRESH, P_KNEE, P_ISOLATE, P_LIGHTSIZE, P_INTENSITY, P_START, P_REACH, P_DEPTH, P_SPREAD, P_FAN, P_GLOW,
    P_SRCCOL, P_TINT,
    P_TOP, P_BOTTOM, P_LEFT, P_RIGHT,
    P_ENCODING,
    P_COUNT
};
enum PType { kDbl, kBool, kChoice, kRGB, kInt };

struct PDef {
    P id;
    const char* name;
    PType type;
    const char* group;
    const char* label;
    const char* hint;
    double def[3];
    double lo, hi, dlo, dhi;
    std::vector<const char*> options = {};  // choices (empty: the encodings list)
};

const std::vector<PDef>& defs() {
    static const std::vector<PDef> d = {
        {P_CROP, "gateCrop", kDbl, nullptr, "Gate Crop (%)",
         "How much of the picture lies beyond the gate. The shot is zoomed in by this much; "
         "bright things in the cropped-off margin flare in from the edge before they enter "
         "and after they leave.",
         {6}, 0, 40, 0, 15},
        {P_CROPOUT, "cropToGate", kBool, nullptr, "Crop to Gate",
         "Scale the gate up to fill the frame. Turn off to keep the full picture (and crop it "
         "yourself).",
         {1}, 0, 1, 0, 1},
        {P_SHOWGATE, "showGate", kBool, nullptr, "Show Gate & Sources",
         "Shows the whole picture with the gate in red, the margin darkened, and the bright "
         "sources that will flare in magenta. For setting up; turn off to render.",
         {0}, 0, 1, 0, 1},

        {P_SHAPE, "shape", kChoice, "grpFlare", "Shape",
         "Plume: a fan that widens into the frame. Beam: a narrow, straight column in from the "
         "edge (Road to Perdition's streak). Blade: a hard-edged wedge that can lean (Angle).",
         {0}, 0, 2, 0, 2, {"Plume (fan)", "Beam (straight column)", "Blade (hard-edged wedge)"}},
        {P_BLADEANGLE, "bladeAngle", kDbl, "grpFlare", "Blade Angle",
         "Blade: degrees it leans away from straight in (negative leans the other way).", {15}, -75,
         75, -45, 45},
        {P_BLADESOFT, "bladeSoftness", kDbl, "grpFlare", "Blade Softness (%)",
         "Blade: how soft its sides are, as % of frame height. 0 = razor sharp.", {0.3}, 0, 10, 0, 3},
        {P_COLOURMODE, "flareColour", kChoice, "grpFlare", "Flare Colour",
         "Source: the colour of the lights causing it. Cool Blue / Cyan / Warm: the tinted flares "
         "of coated lenses and gates. Custom: white, coloured by Tint. Tint applies on top of all "
         "of them.",
         {0}, 0, 4, 0, 4, {"Source", "Cool Blue", "Cyan", "Warm", "Custom (Tint)"}},
        {P_COMPOSITE, "composite", kChoice, "grpFlare", "Composite",
         "Film: the flare is extra exposure on the negative, through a soft shoulder: it glows "
         "into the highlights and never clips. Screen: brighter, but strong flares flatten to "
         "white. Add: plain linear add, for log / scene-linear footage that goes through an "
         "output transform afterwards.",
         {0}, 0, 2, 0, 2, {"Film (soft shoulder)", "Screen", "Add (linear)"}},
        {P_THRESH, "threshold", kDbl, "grpFlare", "Source Threshold",
         "How bright (as encoded) something must be to flare. Lower = more things flare. Use "
         "Show Gate & Sources to see what's picked up.",
         {0.9}, 0, 1.5, 0.3, 1.1},
        {P_KNEE, "knee", kDbl, "grpFlare", "Softness", "Softens the threshold.", {0.05}, 0, 0.6, 0,
         0.3},
        {P_ISOLATE, "isolateLights", kDbl, "grpFlare", "Isolate Lights",
         "1 = only small things much brighter than their surroundings flare (lamps, practicals, "
         "speculars); big bright areas like lit walls don't. 0 = anything over the threshold "
         "flares, including windows and walls.",
         {1}, 0, 1, 0, 1},
        {P_LIGHTSIZE, "lightSize", kDbl, "grpFlare", "Light Size (%)",
         "With Isolate Lights: the biggest thing that still counts as a light, as % of frame "
         "height. Raise it for big practicals or windows.",
         {3}, 0.2, 50, 0.5, 20},
        {P_INTENSITY, "intensity", kDbl, "grpFlare", "Intensity",
         "Strength of the flare. (Lights are usually clipped in the footage, so they get a "
         "built-in boost that tapers off for lights that are genuinely very bright in "
         "scene-linear footage.)",
         {1}, 0, 20, 0, 4},
        {P_START, "flareStart", kDbl, "grpFlare", "Flare Start (%)",
         "How far from the gate edge a light starts to flare, as % of frame height. 0 = as it "
         "reaches the edge. Positive = earlier, while it's still inside the frame. Negative = "
         "later, only once it's past the edge.",
         {0}, -20, 10, -6, 4},
        {P_REACH, "reach", kDbl, "grpFlare", "Reach (%)",
         "How far beyond the gate edge a light keeps flaring (it fades out over this), as % of "
         "frame height.", {6}, 0.5,
         40, 1, 15},
        {P_DEPTH, "depth", kDbl, "grpFlare", "Depth (%)",
         "How far the flare reaches into the frame from the edge, as % of frame height.", {14},
         1, 100, 2, 50},
        {P_SPREAD, "spread", kDbl, "grpFlare", "Spread (%)",
         "Width of the flare along the edge where it starts, as % of frame height.", {2.5}, 0, 30,
         0, 10},
        {P_FAN, "fan", kDbl, "grpFlare", "Fan",
         "How much wider the flare gets as it travels into the frame.", {0.6}, 0, 4, 0, 2},
        {P_GLOW, "edgeGlow", kDbl, "grpFlare", "Edge Glow",
         "A thin glow right along the gate edge, where the light hits it.", {0.35}, 0, 5, 0, 2},
        {P_SRCCOL, "sourceColour", kDbl, "grpFlare", "Source Colour",
         "1 = the flare takes the colour of the lights causing it; 0 = white.", {1}, 0, 1, 0, 1},
        {P_TINT, "tint", kRGB, "grpFlare", "Tint", "Colours the flare.", {1, 1, 1}, 0, 2, 0, 1},

        {P_STREAKS, "streaks", kDbl, "grpCharacter", "Streaks",
         "Striations in the flare from the gate edge's nicks and roughness, running in the "
         "flare's direction. 0 = a smooth glow.",
         {0.5}, 0, 2, 0, 1},
        {P_STREAKDETAIL, "streakDetail", kDbl, "grpCharacter", "Streak Detail",
         "0 = broad streaks, 1 = fine ones.", {0.5}, 0, 1, 0, 1},
        {P_SHIMMER, "shimmer", kDbl, "grpCharacter", "Shimmer",
         "How much the streaks and the flare change from frame to frame, like light catching a "
         "rough edge as it moves. 0 = still.",
         {0.3}, 0, 1, 0, 1},
        {P_SHIMMERSPEED, "shimmerSpeed", kDbl, "grpCharacter", "Shimmer Speed", "How fast it shimmers.",
         {1}, 0, 10, 0, 4},
        {P_CORE, "core", kDbl, "grpCharacter", "Core",
         "A hot, tight core right at the edge with a long subtle tail (1), versus one even falloff "
         "(0).",
         {0.4}, 0, 1, 0, 1},
        {P_FRINGE, "fringe", kDbl, "grpCharacter", "Fringe",
         "Colour fringing: red scatters a little wider than blue.", {0.3}, 0, 2, 0, 1},
        {P_SEED, "seed", kInt, "grpCharacter", "Seed", "A different streak pattern.", {1}, 0, 100000,
         0, 100},
        {P_TOP, "edgeTop", kBool, "grpEdges", "Top", "Flare from the top edge.", {1}, 0, 1, 0, 1},
        {P_BOTTOM, "edgeBottom", kBool, "grpEdges", "Bottom", "Flare from the bottom edge.", {1}, 0,
         1, 0, 1},
        {P_LEFT, "edgeLeft", kBool, "grpEdges", "Left", "Flare from the left edge.", {1}, 0, 1, 0, 1},
        {P_RIGHT, "edgeRight", kBool, "grpEdges", "Right", "Flare from the right edge.", {1}, 0, 1, 0,
         1},

        {P_ENCODING, "encoding", kChoice, "grpInput", "Input Encoding",
         "How the images arriving here are encoded, so the flare is added in linear light. "
         "Rec.709 for a standard timeline; DaVinci Intermediate for DaVinci Wide Gamut; the "
         "camera log if the effect sees log footage.",
         {0}, 0, encodings::kEncodingCount - 1, 0, encodings::kEncodingCount - 1},
    };
    return d;
}

struct GDef {
    const char* name;
    const char* label;
    bool open;
};
const GDef kGroups[] = {{"grpFlare", "Flare", true}, {"grpCharacter", "Character", true}, {"grpEdges", "Edges", false}, {"grpInput", "Input", false}};

const OfxPropertySuiteV1* gProp = nullptr;
const OfxImageEffectSuiteV1* gFx = nullptr;
const OfxParameterSuiteV1* gParam = nullptr;
OfxHost* gHost = nullptr;

struct Instance {
    OfxImageClipHandle src = nullptr, dst = nullptr;
    OfxParamHandle h[P_COUNT] = {};
};

Instance* getInstance(OfxImageEffectHandle e) {
    OfxPropertySetHandle props;
    gFx->getPropertySet(e, &props);
    void* p = nullptr;
    gProp->propGetPointer(props, kOfxPropInstanceData, 0, &p);
    return static_cast<Instance*>(p);
}

gate::Settings read(Instance* in, double t) {
    double v[P_COUNT][3] = {};
    for (const PDef& d : defs()) {
        for (int i = 0; i < 3; ++i) v[d.id][i] = d.def[i];
        if (!in->h[d.id]) continue;
        if (d.type == kDbl) gParam->paramGetValueAtTime(in->h[d.id], t, &v[d.id][0]);
        else if (d.type == kRGB) gParam->paramGetValueAtTime(in->h[d.id], t, &v[d.id][0], &v[d.id][1], &v[d.id][2]);
        else {
            int x = int(d.def[0]);
            if (gParam->paramGetValueAtTime(in->h[d.id], t, &x) == kOfxStatOK) v[d.id][0] = x;
        }
    }
    gate::Settings s;
    s.crop = float(v[P_CROP][0]);
    s.cropOutput = v[P_CROPOUT][0] != 0;
    s.showGate = v[P_SHOWGATE][0] != 0;
    s.threshold = float(v[P_THRESH][0]);
    s.knee = float(v[P_KNEE][0]);
    s.isolate = float(v[P_ISOLATE][0]);
    s.lightSize = float(v[P_LIGHTSIZE][0]);
    s.intensity = float(v[P_INTENSITY][0]);
    s.reach = float(v[P_REACH][0]);
    s.flareStart = float(v[P_START][0]);
    s.depth = float(v[P_DEPTH][0]);
    s.spread = float(v[P_SPREAD][0]);
    s.fan = float(v[P_FAN][0]);
    s.edgeGlow = float(v[P_GLOW][0]);
    s.sourceColour = float(v[P_SRCCOL][0]);
    for (int i = 0; i < 3; ++i) s.tint[i] = float(v[P_TINT][i]);
    s.edges[0] = v[P_TOP][0] != 0;
    s.edges[1] = v[P_BOTTOM][0] != 0;
    s.edges[2] = v[P_LEFT][0] != 0;
    s.edges[3] = v[P_RIGHT][0] != 0;
    s.encoding = std::min(std::max(int(v[P_ENCODING][0]), 0), int(encodings::kEncodingCount) - 1);
    s.shape = std::min(std::max(int(v[P_SHAPE][0]), 0), 2);
    s.composite = std::min(std::max(int(v[P_COMPOSITE][0]), 0), 2);
    s.bladeAngle = float(v[P_BLADEANGLE][0]);
    s.bladeSoftness = float(v[P_BLADESOFT][0]);
    s.streaks = float(v[P_STREAKS][0]);
    s.streakDetail = float(v[P_STREAKDETAIL][0]);
    s.shimmer = float(v[P_SHIMMER][0]);
    s.shimmerSpeed = float(v[P_SHIMMERSPEED][0]);
    s.core = float(v[P_CORE][0]);
    s.fringe = float(v[P_FRINGE][0]);
    s.seed = unsigned(std::max(0, int(v[P_SEED][0])));
    s.time = float(t);
    // Flare Colour presets: replace the source's colour, then Tint on top.
    static const float presets[5][3] = {{1, 1, 1}, {0.55f, 0.72f, 1.f}, {0.55f, 0.95f, 1.f}, {1.f, 0.75f, 0.5f}, {1, 1, 1}};
    const int cm = std::min(std::max(int(v[P_COLOURMODE][0]), 0), 4);
    if (cm != 0) s.sourceColour = 0.f;
    for (int i = 0; i < 3; ++i) s.tint[i] *= presets[cm][i];
    s.upSign = +1;  // OFX images are bottom-up
    return s;
}

bool identity(const gate::Settings& s) {
    const bool noFlare = s.intensity <= 0.f && s.edgeGlow <= 0.f;
    const bool noCrop = s.crop <= 0.f || !s.cropOutput;
    return noFlare && noCrop && !s.showGate;
}

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
    gProp->propSetString(p, kOfxPropLabel, 0, "GateFlare");
    gProp->propSetString(p, kOfxImageEffectPluginPropGrouping, 0, "Lumpy");
    gProp->propSetString(p, kOfxPropPluginDescription, 0,
                         "Film-gate edge flare: bright sources just outside the frame flare in "
                         "from the edge of the gate as they enter or leave the shot. Crops in "
                         "slightly so the margin can act as the area beyond the gate.");
    gProp->propSetString(p, kOfxImageEffectPropSupportedContexts, 0, kOfxImageEffectContextFilter);
    gProp->propSetString(p, kOfxImageEffectPropSupportedContexts, 1, kOfxImageEffectContextGeneral);
    gProp->propSetString(p, kOfxImageEffectPropSupportedPixelDepths, 0, kOfxBitDepthFloat);
    gProp->propSetInt(p, kOfxImageEffectPropSupportsTiles, 0, 0);
    gProp->propSetInt(p, kOfxImageEffectPropTemporalClipAccess, 0, 0);
    gProp->propSetInt(p, kOfxImageEffectPropSupportsMultiResolution, 0, 1);
    gProp->propSetInt(p, kOfxImageEffectPropSupportsMultipleClipDepths, 0, 0);
    gProp->propSetInt(p, kOfxImageEffectPluginPropHostFrameThreading, 0, 0);
    gProp->propSetString(p, kOfxImageEffectPluginRenderThreadSafety, 0, kOfxImageEffectRenderFullySafe);
    return kOfxStatOK;
}

OfxStatus describeInContext(OfxImageEffectHandle effect, OfxPropertySetHandle) {
    OfxPropertySetHandle c;
    gFx->clipDefine(effect, kOfxImageEffectSimpleSourceClipName, &c);
    gProp->propSetString(c, kOfxImageEffectPropSupportedComponents, 0, kOfxImageComponentRGBA);
    gProp->propSetInt(c, kOfxImageEffectPropSupportsTiles, 0, 0);
    gFx->clipDefine(effect, kOfxImageEffectOutputClipName, &c);
    gProp->propSetString(c, kOfxImageEffectPropSupportedComponents, 0, kOfxImageComponentRGBA);
    gProp->propSetInt(c, kOfxImageEffectPropSupportsTiles, 0, 0);

    OfxParamSetHandle ps;
    gFx->getParamSet(effect, &ps);
    const char* openGroup = nullptr;
    for (const PDef& d : defs()) {
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
        static const char* const types[] = {kOfxParamTypeDouble, kOfxParamTypeBoolean, kOfxParamTypeChoice,
                                            kOfxParamTypeRGB, kOfxParamTypeInteger};
        OfxPropertySetHandle p;
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
            case kBool:
                gProp->propSetInt(p, kOfxParamPropDefault, 0, int(d.def[0]));
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
                if (d.options.empty())
                    for (int i = 0; i < encodings::kEncodingCount; ++i)
                        gProp->propSetString(p, kOfxParamPropChoiceOption, i, encodings::names()[i]);
                else
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
    gFx->clipGetHandle(effect, kOfxImageEffectSimpleSourceClipName, &in->src, nullptr);
    gFx->clipGetHandle(effect, kOfxImageEffectOutputClipName, &in->dst, nullptr);
    OfxParamSetHandle ps;
    gFx->getParamSet(effect, &ps);
    for (const PDef& d : defs())
        if (gParam->paramGetHandle(ps, d.name, &in->h[d.id], nullptr) != kOfxStatOK) in->h[d.id] = nullptr;
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

OfxStatus isIdentity(OfxImageEffectHandle effect, OfxPropertySetHandle inArgs, OfxPropertySetHandle outArgs) {
    Instance* in = getInstance(effect);
    double t = 0;
    gProp->propGetDouble(inArgs, kOfxPropTime, 0, &t);
    if (!identity(read(in, t))) return kOfxStatReplyDefault;
    gProp->propSetString(outArgs, kOfxPropName, 0, kOfxImageEffectSimpleSourceClipName);
    gProp->propSetDouble(outArgs, kOfxPropTime, 0, t);
    return kOfxStatOK;
}

OfxStatus render(OfxImageEffectHandle effect, OfxPropertySetHandle inArgs) {
    Instance* in = getInstance(effect);
    if (!in) return kOfxStatErrBadHandle;
    double t = 0;
    OfxRectI win{0, 0, 0, 0};
    gProp->propGetDouble(inArgs, kOfxPropTime, 0, &t);
    gProp->propGetIntN(inArgs, kOfxImageEffectPropRenderWindow, 4, &win.x1);
    const gate::Settings s = read(in, t);

    OfxPropertySetHandle sh = nullptr;
    if (gFx->clipGetImage(in->src, t, nullptr, &sh) != kOfxStatOK || !sh) return kOfxStatFailed;
    void* sd = nullptr;
    int srb = 0;
    OfxRectI sb;
    char* depth = nullptr;
    char* comps = nullptr;
    gProp->propGetPointer(sh, kOfxImagePropData, 0, &sd);
    gProp->propGetInt(sh, kOfxImagePropRowBytes, 0, &srb);
    gProp->propGetIntN(sh, kOfxImagePropBounds, 4, &sb.x1);
    gProp->propGetString(sh, kOfxImageEffectPropPixelDepth, 0, &depth);
    gProp->propGetString(sh, kOfxImageEffectPropComponents, 0, &comps);
    const int w = sb.x2 - sb.x1, h = sb.y2 - sb.y1;
    if (!sd || w <= 0 || h <= 0 || !depth || !comps || std::strcmp(depth, kOfxBitDepthFloat) ||
        std::strcmp(comps, kOfxImageComponentRGBA)) {
        gFx->clipReleaseImage(sh);
        return kOfxStatErrUnsupported;
    }
    gate::Image img(w, h), out;
    for (int y = 0; y < h; ++y)
        std::memcpy(img.row(y), static_cast<const char*>(sd) + std::ptrdiff_t(y) * srb, size_t(w) * 4 * sizeof(float));
    gFx->clipReleaseImage(sh);
    gate::render(img, s, out);

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
    gate::parallelFor(std::max(0, y2 - y1), [&](int j) {
        const int y = y1 + j;
        float* d = reinterpret_cast<float*>(static_cast<char*>(dd) + std::ptrdiff_t(y - db.y1) * drb) +
                   size_t(x1 - db.x1) * 4;
        const int sy = y - sb.y1;
        for (int x = x1; x < x2; ++x, d += 4) {
            const int sx = x - sb.x1;
            if (sy < 0 || sy >= h || sx < 0 || sx >= w) {
                d[0] = d[1] = d[2] = d[3] = 0.f;
                continue;
            }
            std::memcpy(d, out.row(sy) + size_t(sx) * 4, 4 * sizeof(float));
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
        if (!std::strcmp(action, kOfxImageEffectActionDescribeInContext)) return describeInContext(effect, inArgs);
        if (!std::strcmp(action, kOfxActionCreateInstance)) return createInstance(effect);
        if (!std::strcmp(action, kOfxActionDestroyInstance)) return destroyInstance(effect);
        if (!std::strcmp(action, kOfxImageEffectActionIsIdentity)) return isIdentity(effect, inArgs, outArgs);
        if (!std::strcmp(action, kOfxImageEffectActionRender)) return render(effect, inArgs);
        return kOfxStatReplyDefault;
    } catch (const std::bad_alloc&) {
        return kOfxStatErrMemory;
    } catch (...) {
        return kOfxStatFailed;
    }
}

void setHost(OfxHost* host) { gHost = host; }

OfxPlugin gPlugin = {kOfxImageEffectPluginApi, 1, kPluginId, 1, 5, setHost, mainEntry};

}  // namespace

GF_EXPORT int OfxGetNumberOfPlugins(void) { return 1; }
GF_EXPORT OfxPlugin* OfxGetPlugin(int nth) { return nth == 0 ? &gPlugin : nullptr; }
