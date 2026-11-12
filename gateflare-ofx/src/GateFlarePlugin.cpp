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
    // per edge pair
    P_TLOOKA, P_TLOOKB, P_TBLEND, P_TINT_T, P_TSTART, P_TTINT,
    P_SLOOKA, P_SLOOKB, P_SBLEND, P_SINT, P_SSTART, P_STINT,
    // custom look
    P_SHAPE, P_PINCH, P_LEAN, P_BLADEANGLE, P_BLADESOFT, P_COLOURMODE,
    P_INTENSITY, P_START, P_REACH, P_DEPTH, P_SPREAD, P_FAN, P_GLOW, P_SRCCOL, P_TINT, P_FARTINT,
    P_STREAKS, P_STREAKDETAIL, P_SHIMMER, P_SHIMMERSPEED, P_CORE, P_FRINGE,
    P_FALLOFF, P_CURVE, P_VEIL, P_IRID, P_BANDS, P_ALONG, P_EDGES,
    // sources
    P_THRESH, P_KNEE, P_ISOLATE, P_LIGHTSIZE,
    P_TOP, P_BOTTOM, P_LEFT, P_RIGHT,
    P_COMPOSITE, P_ENCODING, P_SEED,
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

const std::vector<const char*> kLookOptions = {"Custom", "Soft Haze", "Thin Streak", "Cool Column", "Blue Blade",
                                               "Side Blast"};
const char* const kLookHint =
    "Custom: the Custom Look controls below. Soft Haze: a soft, streaky plume in the lights' own colour. "
    "Thin Streak: a long, thin streak straight in (a light leaving the side, Road to Perdition). Cool "
    "Column: a cool column straight in. Blue Blade: a hard-edged blue wedge. Side Blast: the light comes "
    "to a soft focus just inside the edge, crosses and opens into a soft, broken-up blue cone (Gummo).";

const std::vector<PDef>& defs() {
    static const std::vector<PDef> d = {
        {P_CROP, "gateCrop", kDbl, nullptr, "Gate Crop (%)",
         "How much of the picture lies beyond the gate. The shot is zoomed in by this much; bright things "
         "in the cropped-off margin flare in from the edge before they enter and after they leave.",
         {6}, 0, 40, 0, 15},
        {P_CROPOUT, "cropToGate", kBool, nullptr, "Crop to Gate",
         "Scale the gate up to fill the frame. Turn off to keep the full picture (and crop it yourself).",
         {1}, 0, 1, 0, 1},
        {P_SHOWGATE, "showGate", kBool, nullptr, "Show Gate & Sources",
         "Shows the whole picture with the gate in red, the margin darkened, and the lights that will "
         "flare in magenta. For setting up; turn off to render.",
         {0}, 0, 1, 0, 1},

        {P_TLOOKA, "topLookA", kChoice, "grpTop", "Look A", kLookHint, {0}, 0, 5, 0, 5, kLookOptions},
        {P_TLOOKB, "topLookB", kChoice, "grpTop", "Look B", kLookHint, {1}, 0, 5, 0, 5, kLookOptions},
        {P_TBLEND, "topBlend", kDbl, "grpTop", "Blend A > B", "0 = all Look A, 1 = all Look B, between = a mix.",
         {0}, 0, 1, 0, 1},
        {P_TINT_T, "topIntensity", kDbl, "grpTop", "Intensity", "Strength of the top and bottom flares.", {1}, 0,
         10, 0, 3},
        {P_TSTART, "topStartOffset", kDbl, "grpTop", "Flare Start Offset (%)",
         "Added to the looks' Flare Start: negative = flare later (only once the light is past the edge).",
         {0}, -20, 10, -5, 3},
        {P_TTINT, "topTint", kRGB, "grpTop", "Tint", "Colours the top and bottom flares.", {1, 1, 1}, 0, 2, 0, 1},

        {P_SLOOKA, "sideLookA", kChoice, "grpSide", "Look A", kLookHint, {0}, 0, 5, 0, 5, kLookOptions},
        {P_SLOOKB, "sideLookB", kChoice, "grpSide", "Look B", kLookHint, {5}, 0, 5, 0, 5, kLookOptions},
        {P_SBLEND, "sideBlend", kDbl, "grpSide", "Blend A > B", "0 = all Look A, 1 = all Look B, between = a mix.",
         {0}, 0, 1, 0, 1},
        {P_SINT, "sideIntensity", kDbl, "grpSide", "Intensity", "Strength of the left and right flares.", {1}, 0,
         10, 0, 3},
        {P_SSTART, "sideStartOffset", kDbl, "grpSide", "Flare Start Offset (%)",
         "Added to the looks' Flare Start: negative = flare later (only once the light is past the edge).",
         {0}, -20, 10, -5, 3},
        {P_STINT, "sideTint", kRGB, "grpSide", "Tint", "Colours the left and right flares.", {1, 1, 1}, 0, 2, 0, 1},

        {P_SHAPE, "shape", kChoice, "grpCustom", "Shape",
         "Plume: a fan that widens into the frame. Beam: a narrow, straight column in from the edge. "
         "Blade: a hard-edged wedge that can lean (Angle). Pinch: a real focus: the light converges "
         "from the edge to a focal point (Pinch Length in), crosses and diverges again at the same angle "
         "(Fan); Spread sets how tight the focus is. Like the side flares in Gummo.",
         {0}, 0, 3, 0, 3,
         {"Plume (fan)", "Beam (straight column)", "Blade (hard-edged wedge)", "Pinch (neck, then fan)"}},
        {P_PINCH, "pinchLength", kDbl, "grpCustom", "Pinch Length (%)",
         "Pinch: how far into the frame the focal point is.", {15}, 0, 100, 0, 40},
        {P_LEAN, "lean", kDbl, "grpCustom", "Lean",
         "Pinch: degrees the whole hourglass leans (positive = up on the left and right edges, left on the top and "
         "bottom).",
         {0}, -60, 60, -30, 30},
        {P_BLADEANGLE, "bladeAngle", kDbl, "grpCustom", "Blade Angle",
         "Blade: degrees it leans away from straight in (negative leans the other way).", {15}, -75, 75, -45,
         45},
        {P_BLADESOFT, "bladeSoftness", kDbl, "grpCustom", "Blade Softness (%)",
         "Blade: how soft its sides are, as % of frame height. 0 = razor sharp.", {0.3}, 0, 10, 0, 3},
        {P_COLOURMODE, "flareColour", kChoice, "grpCustom", "Flare Colour",
         "Source: the lights' own colour. Cool Blue / Cyan / Warm: tinted flares. Custom: white, coloured by "
         "Tint.",
         {0}, 0, 4, 0, 4, {"Source", "Cool Blue", "Cyan", "Warm", "Custom (Tint)"}},
        {P_INTENSITY, "intensity", kDbl, "grpCustom", "Intensity",
         "Strength of the flare. (Lights are usually clipped in the footage, so they get a built-in boost "
         "that tapers off for lights that are genuinely very bright in scene-linear footage.)",
         {1}, 0, 20, 0, 4},
        {P_START, "flareStart", kDbl, "grpCustom", "Flare Start (%)",
         "How far from the gate edge a light starts to flare, as % of frame height. 0 = as it reaches the "
         "edge. Positive = earlier, while it's still inside the frame. Negative = later, once it's past the "
         "edge (only as far as Gate Crop leaves room for).",
         {0}, -20, 10, -6, 4},
        {P_REACH, "reach", kDbl, "grpCustom", "Reach (%)",
         "How far beyond the gate edge a light keeps flaring (it fades out over this).", {6}, 0.5, 40, 1, 15},
        {P_DEPTH, "depth", kDbl, "grpCustom", "Depth (%)", "How far the flare reaches into the frame.", {14}, 1,
         100, 2, 50},
        {P_SPREAD, "spread", kDbl, "grpCustom", "Spread (%)", "Width of the flare along the edge where it starts.",
         {2.5}, 0, 30, 0, 10},
        {P_FAN, "fan", kDbl, "grpCustom", "Fan", "How much wider it gets as it travels into the frame.", {0.6}, 0,
         4, 0, 2},
        {P_GLOW, "edgeGlow", kDbl, "grpCustom", "Edge Glow", "A thin glow right along the gate edge.", {0.35}, 0,
         5, 0, 2},
        {P_SRCCOL, "sourceColour", kDbl, "grpCustom", "Source Colour",
         "With Flare Colour = Source: 1 = the lights' colour, 0 = white.", {1}, 0, 1, 0, 1},
        {P_TINT, "tint", kRGB, "grpCustom", "Tint", "Colours the flare.", {1, 1, 1}, 0, 2, 0, 1},
        {P_FARTINT, "farTint", kRGB, "grpCustom", "Far Tint",
         "The colour the flare shifts toward as it goes into the frame (white = no shift). E.g. deep blue "
         "for a cyan-white core that turns blue.",
         {1, 1, 1}, 0, 2, 0, 1},
        {P_STREAKS, "streaks", kDbl, "grpCustom", "Streaks",
         "Striations from the gate edge's nicks and roughness, radiating from the light (Pinch: rays through "
         "the pinch). 0 = smooth.",
         {0.5}, 0, 2, 0, 2},
        {P_STREAKDETAIL, "streakDetail", kDbl, "grpCustom", "Streak Detail", "0 = broad streaks, 1 = fine ones.",
         {0.5}, 0, 1, 0, 1},
        {P_SHIMMER, "shimmer", kDbl, "grpCustom", "Shimmer",
         "How much the streaks and the flare change from frame to frame. 0 = still.", {0.3}, 0, 1, 0, 1},
        {P_SHIMMERSPEED, "shimmerSpeed", kDbl, "grpCustom", "Shimmer Speed", "How fast it shimmers.", {1}, 0, 10,
         0, 4},
        {P_CORE, "core", kDbl, "grpCustom", "Core",
         "A hot, tight core at the edge with a long subtle tail (1), versus one even falloff (0).", {0.4}, 0, 1,
         0, 1},
        {P_FRINGE, "fringe", kDbl, "grpCustom", "Fringe", "Colour fringing: red scatters a little wider than blue.",
         {0.3}, 0, 2, 0, 1},
        {P_FALLOFF, "falloff", kDbl, "grpCustom", "Falloff",
         "0: the flare fades gradually from the edge. 1: it holds its strength, then drops away at about Depth.",
         {0}, 0, 1, 0, 1},
        {P_CURVE, "curve", kDbl, "grpCustom", "Curve",
         "Bends the flare as it goes into the frame, into an arc (positive bends the same way as Lean).", {0},
         -3, 3, -1.5, 1.5},
        {P_VEIL, "veil", kDbl, "grpCustom", "Veil",
         "Veiling glare: a wide, low haze around the flare that lifts the blacks.", {0}, 0, 5, 0, 2},
        {P_IRID, "iridescence", kDbl, "grpCustom", "Iridescence",
         "Rainbow banding, like thin-film or prism colours. Keeps the brightness, shifts the hue.", {0}, 0, 1,
         0, 1},
        {P_BANDS, "rainbowBands", kDbl, "grpCustom", "Rainbow Bands", "How many rainbow bands.", {3}, 0.2, 20,
         0.5, 10},
        {P_ALONG, "rainbowAlong", kDbl, "grpCustom", "Rainbow Direction",
         "0: bands across the depth, like thin-film colours. 1: bands running along the flare, parallel to it "
         "(with Curve: rainbow arcs).",
         {0}, 0, 1, 0, 1},
        {P_EDGES, "prismEdges", kDbl, "grpCustom", "Prism Edges",
         "1: the rainbow only on the rays' edges, as thin spectral lines (a prism). 0: everywhere.", {0}, 0, 1, 0,
         1},

        {P_THRESH, "threshold", kDbl, "grpSources", "Source Threshold",
         "How bright (as encoded) something must be to flare. Use Show Gate & Sources to see what's picked "
         "up.",
         {0.9}, 0, 1.5, 0.3, 1.1},
        {P_KNEE, "knee", kDbl, "grpSources", "Softness", "Softens the threshold.", {0.05}, 0, 0.6, 0, 0.3},
        {P_ISOLATE, "isolateLights", kDbl, "grpSources", "Isolate Lights",
         "1 = only small things much brighter than their surroundings flare (lamps, practicals); big bright "
         "areas like lit walls don't. 0 = anything over the threshold flares.",
         {1}, 0, 1, 0, 1},
        {P_LIGHTSIZE, "lightSize", kDbl, "grpSources", "Light Size (%)",
         "With Isolate Lights: the biggest thing that still counts as a light, as % of frame height.", {3}, 0.2,
         50, 0.5, 20},

        {P_TOP, "edgeTop", kBool, "grpEdges", "Top", "Flare from the top edge.", {1}, 0, 1, 0, 1},
        {P_BOTTOM, "edgeBottom", kBool, "grpEdges", "Bottom", "Flare from the bottom edge.", {1}, 0, 1, 0, 1},
        {P_LEFT, "edgeLeft", kBool, "grpEdges", "Left", "Flare from the left edge.", {1}, 0, 1, 0, 1},
        {P_RIGHT, "edgeRight", kBool, "grpEdges", "Right", "Flare from the right edge.", {1}, 0, 1, 0, 1},

        {P_COMPOSITE, "composite", kChoice, "grpInput", "Composite",
         "Film: the flare is extra exposure on the negative, through a soft shoulder: it glows into the "
         "highlights and never clips. Screen: brighter, but strong flares flatten to white. Add: plain linear "
         "add, for log / scene-linear footage that goes through an output transform afterwards.",
         {0}, 0, 2, 0, 2, {"Film (soft shoulder)", "Screen", "Add (linear)"}},
        {P_ENCODING, "encoding", kChoice, "grpInput", "Input Encoding",
         "How the images arriving here are encoded, so the flare is added in linear light.", {0}, 0,
         encodings::kEncodingCount - 1, 0, encodings::kEncodingCount - 1},
        {P_SEED, "seed", kInt, "grpInput", "Seed", "A different streak pattern.", {1}, 0, 100000, 0, 100},
    };
    return d;
}

struct GDef {
    const char* name;
    const char* label;
    bool open;
};
const GDef kGroups[] = {{"grpTop", "Top & Bottom", true},     {"grpSide", "Left & Right", true},
                        {"grpCustom", "Custom Look", false},  {"grpSources", "Sources", false},
                        {"grpEdges", "Edges", false},         {"grpInput", "Composite & Input", false}};

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
    auto f = [&](P p, int i = 0) { return float(v[p][i]); };
    gate::Settings s;
    s.crop = f(P_CROP);
    s.cropOutput = v[P_CROPOUT][0] != 0;
    s.showGate = v[P_SHOWGATE][0] != 0;
    s.threshold = f(P_THRESH);
    s.knee = f(P_KNEE);
    s.isolate = f(P_ISOLATE);
    s.lightSize = f(P_LIGHTSIZE);
    s.edges[0] = v[P_TOP][0] != 0;
    s.edges[1] = v[P_BOTTOM][0] != 0;
    s.edges[2] = v[P_LEFT][0] != 0;
    s.edges[3] = v[P_RIGHT][0] != 0;
    s.encoding = std::min(std::max(int(v[P_ENCODING][0]), 0), int(encodings::kEncodingCount) - 1);
    s.composite = std::min(std::max(int(v[P_COMPOSITE][0]), 0), 2);
    s.seed = unsigned(std::max(0, int(v[P_SEED][0])));
    s.time = float(t);
    s.upSign = +1;  // OFX images are bottom-up

    // The Custom Look.
    gate::Look c;
    c.shape = std::min(std::max(int(v[P_SHAPE][0]), 0), 3);
    c.pinch = f(P_PINCH);
    c.lean = f(P_LEAN);
    for (int i = 0; i < 3; ++i) c.farTint[i] = f(P_FARTINT, i);
    c.falloff = f(P_FALLOFF);
    c.curve = f(P_CURVE);
    c.veil = f(P_VEIL);
    c.iridescence = f(P_IRID);
    c.rainbowBands = f(P_BANDS);
    c.rainbowAlong = f(P_ALONG);
    c.prismEdges = f(P_EDGES);
    c.intensity = f(P_INTENSITY);
    c.flareStart = f(P_START);
    c.reach = f(P_REACH);
    c.depth = f(P_DEPTH);
    c.spread = f(P_SPREAD);
    c.fan = f(P_FAN);
    c.edgeGlow = f(P_GLOW);
    c.bladeAngle = f(P_BLADEANGLE);
    c.bladeSoftness = f(P_BLADESOFT);
    c.streaks = f(P_STREAKS);
    c.streakDetail = f(P_STREAKDETAIL);
    c.shimmer = f(P_SHIMMER);
    c.shimmerSpeed = f(P_SHIMMERSPEED);
    c.core = f(P_CORE);
    c.fringe = f(P_FRINGE);
    c.sourceColour = f(P_SRCCOL);
    for (int i = 0; i < 3; ++i) c.tint[i] = f(P_TINT, i);
    // Flare Colour presets: replace the source's colour, then Tint on top.
    static const float presets[5][3] = {{1, 1, 1}, {0.55f, 0.72f, 1.f}, {0.55f, 0.95f, 1.f}, {1.f, 0.75f, 0.5f}, {1, 1, 1}};
    const int cm = std::min(std::max(int(v[P_COLOURMODE][0]), 0), 4);
    if (cm != 0) c.sourceColour = 0.f;
    for (int i = 0; i < 3; ++i) c.tint[i] *= presets[cm][i];

    // Each edge pair: two looks, a blend, and its own strength / timing / tint.
    struct PairP { P a, b, blend, inten, start, tint; };
    const PairP pp[2] = {{P_TLOOKA, P_TLOOKB, P_TBLEND, P_TINT_T, P_TSTART, P_TTINT},
                         {P_SLOOKA, P_SLOOKB, P_SBLEND, P_SINT, P_SSTART, P_STINT}};
    for (int k = 0; k < 2; ++k) {
        gate::EdgePair& ep = s.pair[k];
        auto finish = [&](gate::Look L) {
            L.intensity *= f(pp[k].inten);
            L.edgeGlow *= f(pp[k].inten);
            L.flareStart += f(pp[k].start);
            for (int i = 0; i < 3; ++i) L.tint[i] *= f(pp[k].tint, i);
            return L;
        };
        ep.a = finish(gate::presetLook(int(v[pp[k].a][0]), c));
        ep.b = finish(gate::presetLook(int(v[pp[k].b][0]), c));
        ep.blend = f(pp[k].blend);
    }
    return s;
}

bool identity(const gate::Settings& s) {
    bool noFlare = true;
    for (int k = 0; k < 2; ++k) {
        const gate::EdgePair& ep = s.pair[k];
        const bool edgesOn = k == 0 ? (s.edges[0] || s.edges[1]) : (s.edges[2] || s.edges[3]);
        if (!edgesOn) continue;
        if (ep.blend < 1.f && (ep.a.intensity > 0.f || ep.a.edgeGlow > 0.f)) noFlare = false;
        if (ep.blend > 0.f && (ep.b.intensity > 0.f || ep.b.edgeGlow > 0.f)) noFlare = false;
    }
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

OfxPlugin gPlugin = {kOfxImageEffectPluginApi, 1, kPluginId, 1, 9, setHost, mainEntry};

}  // namespace

GF_EXPORT int OfxGetNumberOfPlugins(void) { return 1; }
GF_EXPORT OfxPlugin* OfxGetPlugin(int nth) { return nth == 0 ? &gPlugin : nullptr; }
