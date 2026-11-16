// EriksTV: a datamosh OpenFX image effect for DaVinci Resolve (and other OFX hosts).
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
#include <algorithm>
#include <atomic>
#include <cctype>
#include <cmath>
#include <cstdlib>
#include <cstdint>
#include <cstring>
#include <deque>
#include <list>
#include <memory>
#include <mutex>
#include <new>
#include <string>
#include <vector>

#if defined(__APPLE__)
#include <sys/sysctl.h>
#include <sys/types.h>
#elif !defined(_WIN32)
#include <unistd.h>
#endif

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

// The identifier stays as it was when the plugin was called DataMosh, so
// projects that already use it keep finding it.
const char* const kPluginId = "com.lumpy.DataMosh";
const char* const kGrouping = "Lumpy";
const int kVersionMajor = 1, kVersionMinor = 3;

const char* const kParamSetStart = "setStartHere";
const char* const kParamLook = "look";
const char* const kParamApplyLook = "applyLook";
enum Mode { kModeDrag = 0, kModeBloom = 1 };
// Choice index -> pixels. 4 and 64 were added after 8/16/32, so they come
// last to keep saved projects pointing at the same sizes.
const int kBlockSizes[] = {8, 16, 32, 4, 64};

// ---------------------------------------------------------------- parameters

enum P {
    P_MODE, P_START, P_LENGTH, P_CYCLE, P_SEED,
    P_BLOCK, P_VARIETY, P_RANGE, P_SCALE, P_RESIDUAL, P_HEAL, P_SMOOTH,
    P_REPEAT, P_FREEZE, P_JBACK, P_JFWD, P_REVERSE, P_EVLEN, P_JDIST, P_EVERY,
    P_STUCK, P_SCATTER, P_STREAK, P_FLATTEN, P_HOLD,
    P_CDRIFT, P_BLEED, P_SAT, P_HUE, P_CERR, P_CERRSTR,
    P_MIX, P_DIAG,
    P_COUNT
};

enum PType { kInt, kDbl, kBool, kChoice };

struct PDef {
    P id;
    const char* name;  // stable: saved in projects
    PType type;
    const char* group; // nullptr = top level
    const char* label;
    const char* hint;
    double def, lo, hi, dlo, dhi;
    bool animates;
    std::vector<const char*> options;
};

struct GDef {
    const char* name;
    const char* label;
    bool open;
};

const GDef kGroups[] = {
    {"grpMotion", "Motion", true},
    {"grpTime", "Time Glitches", false},
    {"grpBlocks", "Block Glitches", false},
    {"grpColour", "Colour", false},
};

// In UI order. setStartHere (a button) is inserted after moshStart.
const std::vector<PDef>& paramDefs() {
    static const std::vector<PDef> defs = {
        {P_MODE, "mode", kChoice, nullptr, "Mode",
         "Drag: the frame before Mosh Start is held and the new shot's motion pushes it around "
         "(deleted I-frame). Bloom: Mosh Start's motion is repeated over and over (duplicated "
         "P-frame).",
         0, 0, 1, 0, 1, false, {"Drag (drop I-frame)", "Bloom (repeat P-frame)"}},
        {P_START, "moshStart", kInt, nullptr, "Mosh Start Frame",
         "First frame of the mosh, usually the first frame of the new shot. Easiest to set with "
         "\"Start Mosh Here\".",
         0, -1000000, 1000000, 0, 1000, false, {}},
        {P_LENGTH, "moshLength", kInt, nullptr, "Mosh Length",
         "How many frames the mosh lasts before the picture cleans up (a fresh I-frame).", 48, 1,
         5000, 1, 240, false, {}},
        {P_CYCLE, "cycle", kInt, nullptr, "Re-mosh Every",
         "0 = mosh once. Otherwise a new mosh starts every this many frames after Mosh Start: "
         "the picture cleans up, then breaks again.",
         0, 0, 5000, 0, 240, false, {}},
        {P_SEED, "seed", kInt, nullptr, "Seed",
         "Changes the random pattern of every glitch below. Same seed, same result.", 1, 0,
         1000000, 0, 100, false, {}},

        // Motion
        {P_BLOCK, "blockSize", kChoice, "grpMotion", "Block Size",
         "Base macroblock size. Smaller blocks follow motion closely; bigger blocks are "
         "chunkier and glitchier.",
         1, 0, 4, 0, 4, false, {"8 px", "16 px", "32 px", "4 px", "64 px"}},
        {P_VARIETY, "blockVariety", kDbl, "grpMotion", "Block Variety",
         "0 = every block the same size. Higher mixes 4-64 px blocks like a real encoder: small "
         "on detail, big on flat areas, plus randomness.",
         0, 0, 1, 0, 1, false, {}},
        {P_RANGE, "searchRange", kInt, "grpMotion", "Search Range",
         "How far (pixels) to look for each block's motion. Raise it for fast motion; costs "
         "render time.",
         24, 2, 128, 4, 64, false, {}},
        {P_SCALE, "motionScale", kDbl, "grpMotion", "Motion Scale",
         "Multiplies every motion vector. Above 1 smears faster than the real motion.", 1, 0, 4,
         0, 4, false, {}},
        {P_RESIDUAL, "residual", kDbl, "grpMotion", "Residual Bleed",
         "How much of the new shot's detail leaks through. 0 = pure smear of the old picture, "
         "1 = what a real decoder would do.",
         0.3, 0, 1, 0, 1, false, {}},
        {P_HEAL, "heal", kDbl, "grpMotion", "Heal",
         "Per-frame blend back toward the real picture, so the mosh slowly resolves.", 0, 0, 1,
         0, 1, false, {}},
        {P_SMOOTH, "smoothVectors", kBool, "grpMotion", "Smooth Vectors",
         "Interpolate motion between blocks for a liquid smear instead of blocky tearing.", 0, 0,
         1, 0, 1, false, {}},

        // Time
        {P_REPEAT, "tRepeat", kDbl, "grpTime", "Repeat",
         "Chance per frame that the same motion is applied again for a few frames: the smear "
         "lurches and blooms.",
         0, 0, 1, 0, 0.5, false, {}},
        {P_FREEZE, "tFreeze", kDbl, "grpTime", "Freeze",
         "Chance per frame that the picture holds still for a few frames (a repeated frame).",
         0, 0, 1, 0, 0.5, false, {}},
        {P_JBACK, "tJumpBack", kDbl, "grpTime", "Jump Back",
         "Chance per frame that the motion jumps back in time and replays.", 0, 0, 1, 0, 0.5,
         false, {}},
        {P_JFWD, "tJumpForward", kDbl, "grpTime", "Jump Forward",
         "Chance per frame that the motion skips ahead in one big lurch.", 0, 0, 1, 0, 0.5,
         false, {}},
        {P_REVERSE, "tReverse", kDbl, "grpTime", "Reverse",
         "Chance per frame that the motion runs backwards for a few frames.", 0, 0, 1, 0, 0.5,
         false, {}},
        {P_EVLEN, "tEventLength", kInt, "grpTime", "Event Length",
         "Longest repeat / freeze / reverse, in frames (each one is random up to this).", 6, 1,
         120, 1, 30, false, {}},
        {P_JDIST, "tJumpDistance", kInt, "grpTime", "Jump Distance",
         "Furthest jump back or forward, in frames.", 12, 1, 240, 1, 60, false, {}},
        {P_EVERY, "tEvery", kInt, "grpTime", "Rhythm (every N frames)",
         "0 = events happen at random. Otherwise they can only start every N frames from Mosh "
         "Start; set a chance to 1 to hit every beat.",
         0, 0, 1000, 0, 48, false, {}},

        // Blocks
        {P_STUCK, "gStuck", kDbl, "grpBlocks", "Stuck",
         "Share of blocks that stop updating and hang in place.", 0, 0, 1, 0, 0.5, false, {}},
        {P_SCATTER, "gScatter", kDbl, "grpBlocks", "Scatter",
         "Share of blocks that grab pixels from a random place.", 0, 0, 1, 0, 0.5, false, {}},
        {P_STREAK, "gStreak", kDbl, "grpBlocks", "Drip",
         "Share of blocks that smear their top row downwards into streaks.", 0, 0, 1, 0, 0.5,
         false, {}},
        {P_FLATTEN, "gFlatten", kDbl, "grpBlocks", "Flatten",
         "Share of blocks that collapse to one flat colour.", 0, 0, 1, 0, 0.5, false, {}},
        {P_HOLD, "gHold", kInt, "grpBlocks", "Glitch Hold",
         "How many frames each block glitch lasts.", 6, 1, 240, 1, 48, false, {}},

        // Colour
        {P_CDRIFT, "cChromaDrift", kDbl, "grpColour", "Chroma Drift",
         "Colour moves further than brightness, so colour slides off the shapes.", 0, 0, 1, 0, 1,
         false, {}},
        {P_BLEED, "cBleed", kDbl, "grpColour", "Colour Bleed",
         "Boosts the colour in the residual: wrong colours pile up frame after frame.", 0, 0, 1,
         0, 1, false, {}},
        {P_SAT, "cSaturate", kDbl, "grpColour", "Saturation Build",
         "Saturation grows every frame of the mosh.", 0, 0, 1, 0, 1, false, {}},
        {P_HUE, "cHueDrift", kDbl, "grpColour", "Hue Drift",
         "Degrees the hue rotates every frame of the mosh (negative turns the other way).", 0,
         -45, 45, -10, 10, false, {}},
        {P_CERR, "cErrors", kDbl, "grpColour", "Colour Errors",
         "How often blotches of wrong colour hit, then get dragged along by the motion.", 0, 0,
         1, 0, 1, false, {}},
        {P_CERRSTR, "cErrorStrength", kDbl, "grpColour", "Error Strength",
         "How strong each colour error is.", 0.5, 0, 2, 0, 1, false, {}},

        {P_MIX, "mix", kDbl, nullptr, "Mix", "Blend between the original (0) and the mosh (1).",
         1, 0, 1, 0, 1, true, {}},
        {P_DIAG, "showDiagnostics", kBool, nullptr, "Show Diagnostics",
         "Prints on the picture what the plugin is doing at this frame: the frame number the host gave it, "
         "Mosh Start, where the held frame came from, and whether the host supplied the other frames it "
         "asked for. For troubleshooting; turn off to render.",
         0, 0, 1, 0, 1, false, {}},
    };
    return defs;
}

// Looks: starting points. Applying one resets every Motion / Time / Block /
// Colour control to its default and then sets these. Placement (mode, start,
// length, re-mosh, seed) and Mix are left alone.
struct Look {
    const char* name;
    std::vector<std::pair<P, double>> values;
};
const std::vector<Look>& looks() {
    static const std::vector<Look> l = {
        {"Classic Drag", {}},
        {"Film Melt",
         {{P_BLOCK, 0}, {P_VARIETY, 0.6}, {P_RESIDUAL, 0.6}, {P_STREAK, 0.06}, {P_HOLD, 8},
          {P_CERR, 0.4}, {P_BLEED, 0.4}, {P_CDRIFT, 0.2}}},
        {"Game Tear",
         {{P_BLOCK, 2}, {P_VARIETY, 0.9}, {P_RESIDUAL, 0.4}, {P_FLATTEN, 0.1}, {P_STREAK, 0.08},
          {P_SCATTER, 0.03}, {P_HOLD, 10}, {P_SAT, 0.4}, {P_BLEED, 0.8}}},
        {"Stutter & Jump",
         {{P_RESIDUAL, 0.5}, {P_REPEAT, 0.15}, {P_FREEZE, 0.1}, {P_JBACK, 0.1}, {P_JFWD, 0.08},
          {P_REVERSE, 0.08}, {P_EVLEN, 5}, {P_JDIST, 10}}},
        {"Colour Riot",
         {{P_RESIDUAL, 0.5}, {P_CDRIFT, 0.5}, {P_HUE, 2}, {P_SAT, 0.6}, {P_BLEED, 0.6},
          {P_CERR, 0.6}, {P_CERRSTR, 1.0}}},
        {"Liquid Smear",
         {{P_BLOCK, 1}, {P_SMOOTH, 1}, {P_RESIDUAL, 0.2}, {P_SCALE, 1.5}, {P_CDRIFT, 0.3}}},
    };
    return l;
}

OfxHost* gHost = nullptr;
const OfxPropertySuiteV1* gProp = nullptr;
const OfxImageEffectSuiteV1* gFx = nullptr;
const OfxParameterSuiteV1* gParam = nullptr;
const OfxMessageSuiteV1* gMsg = nullptr;

// ---------------------------------------------------------------- instance

struct Params {
    double v[P_COUNT] = {};
    int i(P p) const { return int(std::lround(v[p])); }
    float f(P p) const { return float(v[p]); }
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
    OfxParamHandle h[P_COUNT] = {};
    OfxParamHandle setStart = nullptr, look = nullptr;

    std::mutex lock;
    std::deque<CachedState> states;  // newest first
    std::list<std::pair<uint64_t, mosh::MotionField>> motion;  // LRU, newest first
    // Real frames this instance was handed as the frame being rendered (the
    // one request every host honours): the last one, and the frames the mosh
    // in progress hangs on (Mosh Start - 1, and Mosh Start for Bloom). Some
    // hosts don't supply other frames reliably (Resolve, on a compound clip,
    // for a while after a cut inside it), so these are the fallback.
    std::list<std::pair<int, Frame>> kept;
    // For the memory budget shared by every instance (see trimCaches).
    std::atomic<size_t> bytes{0};     // what the two caches hold
    std::atomic<uint64_t> lastUse{0};
};

// ------------------------------------------------------------ memory budget
//
// A host makes an instance per clip (often several), and each used to keep
// its own caches and working frames for as long as it lived: hundreds of MB
// each at UHD, gigabytes with mixed block sizes. Now every instance shares
// one budget: when it's exceeded, idle instances give up their caches first,
// then the one rendering drops its oldest entries. The working frames
// (Scratch) are shared too.

size_t bytesOf(const mosh::Image& i) { return i.px.capacity() * sizeof(float); }
size_t bytesOf(const mosh::MotionField& m) {
    return (m.dx.capacity() + m.dy.capacity()) * sizeof(float) + m.blocks.capacity() * sizeof(mosh::Block);
}
size_t cachedBytes(const Instance* in) {
    size_t n = 0;
    for (const CachedState& c : in->states) n += bytesOf(c.img);
    for (const auto& m : in->motion) n += bytesOf(m.second);
    for (const auto& k : in->kept) n += bytesOf(k.second.img);
    return n;
}

struct Registry {
    std::mutex m;
    std::vector<Instance*> all;
    std::vector<mosh::Scratch> scratch;  // at most one spare set of working frames
};
Registry& registry() {
    static Registry r;
    return r;
}
std::atomic<uint64_t> gUseClock{0};

// A sixteenth of the machine's memory, between 256 MB and 1 GB: the host needs
// the rest, and only the newest state of a clip that is playing really pays
// for itself (ERIKSTV_CACHE_MB overrides it).
size_t cacheBudget() {
    static const size_t budget = [] {
        const size_t mb = size_t(1) << 20;
        if (const char* e = std::getenv("ERIKSTV_CACHE_MB")) {
            const long v = std::atol(e);
            if (v > 0) return size_t(v) * mb;
        }
        uint64_t ram = 0;
#if defined(__APPLE__)
        size_t len = sizeof ram;
        if (sysctlbyname("hw.memsize", &ram, &len, nullptr, 0) != 0) ram = 0;
#elif !defined(_WIN32)
        const long pages = sysconf(_SC_PHYS_PAGES), page = sysconf(_SC_PAGE_SIZE);
        if (pages > 0 && page > 0) ram = uint64_t(pages) * uint64_t(page);
#endif
        if (!ram) ram = uint64_t(8) << 30;
        return size_t(std::min<uint64_t>(std::max<uint64_t>(ram / 16, 256 * mb), 1024 * mb));
    }();
    return budget;
}

// Call with `cur`'s lock held, after it has rendered.
void trimCaches(Instance* cur) {
    const size_t budget = cacheBudget();
    // One instance never takes more than half for its motion fields.
    size_t motionBytes = 0;
    for (const auto& m : cur->motion) motionBytes += bytesOf(m.second);
    while (motionBytes > budget / 2 && cur->motion.size() > 4) {
        motionBytes -= bytesOf(cur->motion.back().second);
        cur->motion.pop_back();
    }
    cur->bytes = cachedBytes(cur);
    cur->lastUse = ++gUseClock;

    Registry& r = registry();
    std::lock_guard<std::mutex> g(r.m);
    auto total = [&] {
        size_t n = 0;
        for (Instance* i : r.all) n += i->bytes;
        return n;
    };
    // Idle instances first, the longest idle first. (try_lock: one that is
    // rendering right now is skipped, and can't deadlock with this one.)
    std::vector<Instance*> busy;
    while (total() > budget) {
        Instance* victim = nullptr;
        for (Instance* i : r.all)
            if (i != cur && i->bytes > 0 && std::find(busy.begin(), busy.end(), i) == busy.end() &&
                (!victim || i->lastUse < victim->lastUse))
                victim = i;
        if (!victim) break;
        if (victim->lock.try_lock()) {
            victim->states.clear();
            victim->motion.clear();
            victim->kept.clear();
            victim->bytes = 0;
            victim->lock.unlock();
        } else {
            busy.push_back(victim);
        }
    }
    // Then this one: its oldest motion fields, then its older states. The
    // newest state stays: playing forward needs it.
    while (total() > budget && cur->motion.size() > 4) {
        cur->bytes -= std::min(size_t(cur->bytes), bytesOf(cur->motion.back().second));
        cur->motion.pop_back();
    }
    while (total() > budget && cur->states.size() > 1) {
        cur->bytes -= std::min(size_t(cur->bytes), bytesOf(cur->states.back().img));
        cur->states.pop_back();
    }
}

// The working frames for one render, borrowed from the shared spare.
struct ScratchLoan {
    mosh::Scratch sc;
    ScratchLoan() {
        Registry& r = registry();
        std::lock_guard<std::mutex> g(r.m);
        if (!r.scratch.empty()) {
            sc = std::move(r.scratch.back());
            r.scratch.pop_back();
        }
    }
    ~ScratchLoan() {
        Registry& r = registry();
        std::lock_guard<std::mutex> g(r.m);
        if (r.scratch.empty()) r.scratch.push_back(std::move(sc));
    }
    ScratchLoan(const ScratchLoan&) = delete;
    ScratchLoan& operator=(const ScratchLoan&) = delete;
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
    for (const PDef& d : paramDefs()) {
        p.v[d.id] = d.def;
        if (!in->h[d.id]) continue;
        if (d.type == kDbl) {
            double x = d.def;
            if (gParam->paramGetValueAtTime(in->h[d.id], t, &x) == kOfxStatOK) p.v[d.id] = x;
        } else {
            int x = int(d.def);
            if (gParam->paramGetValueAtTime(in->h[d.id], t, &x) == kOfxStatOK) p.v[d.id] = x;
        }
    }
    p.v[P_LENGTH] = std::max(1, p.i(P_LENGTH));
    return p;
}

// Start of the mosh window `frame` falls in (Re-mosh Every repeats it).
int effectiveStart(const Params& p, int frame) {
    const int start = p.i(P_START), cycle = p.i(P_CYCLE);
    if (cycle > 0 && frame >= start) return start + (frame - start) / cycle * cycle;
    return start;
}

bool inMosh(const Params& p, int frame) {
    const int s = effectiveStart(p, frame);
    return frame >= s && frame < s + p.i(P_LENGTH);
}

int blockPixels(const Params& p) {
    return kBlockSizes[std::min(std::max(p.i(P_BLOCK), 0), 4)];
}

mosh::TimeSettings timeSettings(const Params& p) {
    mosh::TimeSettings ts;
    ts.repeat = p.f(P_REPEAT);
    ts.freeze = p.f(P_FREEZE);
    ts.jumpBack = p.f(P_JBACK);
    ts.jumpFwd = p.f(P_JFWD);
    ts.reverse = p.f(P_REVERSE);
    ts.maxLen = p.i(P_EVLEN);
    ts.maxJump = p.i(P_JDIST);
    ts.every = p.i(P_EVERY);
    ts.seed = uint32_t(p.i(P_SEED));
    return ts;
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

// What one build did (for Show Diagnostics).
struct BuildInfo {
    bool tried = false, ok = false, usedCache = false;
    int start = 0, from = 0;
    int refSource = 0;  // 0 missing, 1 from the host, 2 remembered
    int applied = 0, skipped = 0;
    int fromHost = 0, remembered = 0, missing = 0, gaveCurrent = 0;
};

// The source frames one build touches. The frame being rendered comes from
// the host. Any other frame is asked of the host too, but if the host has
// none, or answers with the frame being rendered instead, the copy this
// instance remembered is used.
struct FrameCache {
    OfxImageClipHandle clip;
    Instance* in;
    int now;
    BuildInfo* info;
    Frame cur;  // the frame being rendered
    std::list<std::pair<int, Frame>> items;  // others, most recent first (above the 3 a step uses at once)
    int lastSource = 0;

    FrameCache(OfxImageClipHandle c, Instance* i, int t, BuildInfo* bi) : clip(c), in(i), now(t), info(bi) {
        fetchFrame(clip, t, cur);
    }
    const Frame* get(int t) {
        if (t == now) {
            lastSource = cur.ok ? 1 : 0;
            return cur.ok ? &cur : nullptr;
        }
        for (auto it = items.begin(); it != items.end(); ++it)
            if (it->first == t) {
                items.splice(items.begin(), items, it);
                return it->second.ok ? &it->second : nullptr;
            }
        if (items.size() >= 5) items.pop_back();
        items.emplace_front(t, Frame{});
        Frame& f = items.front().second;
        fetchFrame(clip, t, f);
        if (f.ok && cur.ok && f.fp == cur.fp) {
            // Another time, the very same picture as the frame being rendered:
            // the host didn't really fetch it (or nothing moved, and then it
            // makes no difference).
            f.ok = false;
            ++info->gaveCurrent;
        }
        if (f.ok) {
            ++info->fromHost;
            lastSource = 1;
            return &f;
        }
        for (auto& k : in->kept)
            if (k.first == t && k.second.ok &&
                (!cur.ok || (k.second.img.w == cur.img.w && k.second.img.h == cur.img.h))) {
                items.pop_front();  // (the empty placeholder)
                ++info->remembered;
                lastSource = 2;
                return &k.second;
            }
        ++info->missing;
        lastSource = 0;
        return nullptr;
    }
};

// After a render: keep the frame just rendered and the frames the mosh hangs on.
void rememberFrame(Instance* in, const Params& p, int t, Frame&& real) {
    if (!real.ok) return;
    const int next = inMosh(p, t) ? t : t + 1;
    const int S = effectiveStart(p, next);
    const bool bloom = p.i(P_MODE) == kModeBloom;
    for (auto it = in->kept.begin(); it != in->kept.end();) {
        const bool anchor = it->first == S - 1 || (bloom && it->first == S);
        it = (it->first == t || !anchor) ? in->kept.erase(it) : std::next(it);
    }
    in->kept.emplace_front(t, std::move(real));
}

const mosh::MotionField& motionFor(Instance* in, const Frame& cur, const Frame& prev,
                                   const mosh::MotionSettings& ms) {
    uint64_t key = hashMix(hashMix(hashMix(hashMix(1, cur.fp), prev.fp), ms.block), ms.range);
    key = hashDouble(hashMix(key, ms.seed), ms.variety);
    for (auto it = in->motion.begin(); it != in->motion.end(); ++it) {
        if (it->first == key) {
            in->motion.splice(in->motion.begin(), in->motion, it);
            return in->motion.front().second;
        }
    }
    in->motion.emplace_front(key, mosh::estimateMotion(cur.img, prev.img, ms));
    while (in->motion.size() > 400) in->motion.pop_back();
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

// Builds the moshed picture for frame t. On success `out` holds it and `real`
// holds the untouched source frame at t (for Mix).
bool buildMosh(OfxImageEffectHandle effect, Instance* in, const Params& p, int t,
               double renderScale, mosh::Image& out, Frame& real, BuildInfo& info) {
    mosh::MotionSettings ms;
    ms.block = std::max(4, int(std::lround(blockPixels(p) * renderScale)));
    ms.range = std::max(2, int(std::lround(p.i(P_RANGE) * renderScale)));
    ms.variety = p.f(P_VARIETY);
    ms.seed = uint32_t(p.i(P_SEED));
    mosh::StepSettings ss;
    ss.motionScale = p.f(P_SCALE);
    ss.residual = p.f(P_RESIDUAL);
    ss.heal = p.f(P_HEAL);
    ss.smooth = p.i(P_SMOOTH) != 0;
    ss.range = ms.range;
    ss.seed = ms.seed;
    ss.glitch.stuck = p.f(P_STUCK);
    ss.glitch.scatter = p.f(P_SCATTER);
    ss.glitch.streak = p.f(P_STREAK);
    ss.glitch.flatten = p.f(P_FLATTEN);
    ss.glitch.hold = p.i(P_HOLD);
    ss.glitch.colourErr = p.f(P_CERR);
    ss.glitch.colourErrStrength = p.f(P_CERRSTR);
    ss.glitch.upSign = +1;  // OFX images are bottom-up
    ss.colour.chromaDrift = p.f(P_CDRIFT);
    ss.colour.bleed = p.f(P_BLEED);
    ss.colour.satBuild = p.f(P_SAT);
    ss.colour.hueDrift = p.f(P_HUE);

    const int S = effectiveStart(p, t);
    uint64_t base = 7;
    for (int i = 0; i < P_COUNT; ++i)
        if (i != P_MIX && i != P_START && i != P_DIAG) base = hashDouble(base, p.v[i]);
    base = hashMix(hashMix(hashMix(base, uint64_t(int64_t(S))), uint64_t(ms.block)),
                   uint64_t(ms.range));

    info.tried = true;
    info.start = info.from = S;
    FrameCache fc(in->src, in, t, &info);
    ScratchLoan loan;
    mosh::Scratch& sc = loan.sc;

    if (p.i(P_MODE) == kModeBloom) {
        // Freeze frame S and keep re-applying its motion.
        const Frame* fz = fc.get(S);
        if (!fz) return false;
        Frame frozen = *fz;
        const Frame* before = fc.get(S - 1);
        info.refSource = fc.lastSource;
        const bool haveBefore =
            before && before->img.w == frozen.img.w && before->img.h == frozen.img.h;
        mosh::MotionField none;
        const mosh::MotionField& mf = haveBefore ? motionFor(in, frozen, *before, ms) : none;
        const uint64_t key = hashMix(hashMix(base, frozen.fp), haveBefore ? before->fp : 0);

        int from = S;
        mosh::Image state = frozen.img;
        if (CachedState* c = findState(in, key, S, t)) {
            from = c->time;
            state = c->img;
            info.usedCache = true;
        }
        info.from = from;
        for (int k = from + 1; k <= t; ++k) {
            const Frame* cur = ss.heal > 0.f ? fc.get(k) : nullptr;
            mosh::step(state, nullptr, nullptr, cur ? &cur->img : nullptr, mf, ss, k, sc);
            ++info.applied;
            if (gFx->abort(effect)) return false;
        }
        const Frame* r = fc.get(t);
        real = r ? *r : frozen;
        storeState(in, key, t, real.fp, state);
        out = std::move(state);
        info.ok = true;
        return true;
    }

    // Drag: frame S is the dropped I-frame, so it shows the frame before it;
    // later frames apply motion (and residual) from the frames the schedule
    // picks: normally frame k vs k-1, or repeated / reversed / jumped.
    const Frame* rf = fc.get(S - 1);
    info.refSource = fc.lastSource;
    if (!rf) rf = fc.get(S);
    if (!rf) return false;
    const Frame ref = *rf;
    const uint64_t key = hashMix(base, ref.fp);
    const std::vector<mosh::StepSpec> plan = mosh::schedule(S, t, timeSettings(p));

    int from = S;
    mosh::Image state = ref.img;
    if (CachedState* c = findState(in, key, S, t)) {
        // Don't trust the cache if the footage at that frame has changed.
        // (If the frame can't be had at all, there's nothing to check it with.)
        const Frame* at = fc.get(c->time);
        if (!at || at->fp == c->fpAtTime) {
            from = c->time;
            state = c->img;
            info.usedCache = true;
        } else {
            in->states.clear();
        }
    }
    info.from = from;

    for (int k = from + 1; k <= t; ++k) {
        const mosh::StepSpec& sp = plan[size_t(k - S - 1)];
        if (sp.move) {
            const Frame* a = fc.get(sp.a);
            const Frame* b = a ? fc.get(sp.b) : nullptr;
            if (a && b && a->img.w == state.w && a->img.h == state.h && b->img.w == state.w &&
                b->img.h == state.h) {
                const mosh::MotionField& mf = motionFor(in, *a, *b, ms);
                const Frame* hl = ss.heal > 0.f ? fc.get(k) : nullptr;
                mosh::step(state, &a->img, &b->img, hl ? &hl->img : nullptr, mf, ss, k, sc);
                ++info.applied;
            } else {
                ++info.skipped;  // a frame it needed wasn't to be had
            }
        }
        if (gFx->abort(effect)) return false;
    }
    const Frame* r = fc.get(t);
    real = r ? *r : ref;
    storeState(in, key, t, real.fp, state);
    out = std::move(state);
    info.ok = true;
    return true;
}

// True on the frame before a mosh starts: it is rendered (passed through) so
// the plugin sees the frame the mosh will hold.
bool armsMosh(const Params& p, int frame) {
    return !inMosh(p, frame) && inMosh(p, frame + 1) && effectiveStart(p, frame + 1) == frame + 1;
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
    gProp->propSetString(p, kOfxPropLabel, 0, "EriksTV");
    gProp->propSetString(p, kOfxImageEffectPluginPropGrouping, 0, kGrouping);
    gProp->propSetString(p, kOfxPropPluginDescription, 0,
                         "Simulated datamosh: drops the I-frame at Mosh Start so the next "
                         "shot's motion drags the previous picture around (Drag), or repeats "
                         "one frame's motion over and over (Bloom). Plus time, block and "
                         "colour glitches.");
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

void defineParam(OfxParamSetHandle ps, const PDef& d) {
    static const char* const types[] = {kOfxParamTypeInteger, kOfxParamTypeDouble,
                                        kOfxParamTypeBoolean, kOfxParamTypeChoice};
    OfxPropertySetHandle p;
    gParam->paramDefine(ps, types[d.type], d.name, &p);
    gProp->propSetString(p, kOfxPropLabel, 0, d.label);
    gProp->propSetString(p, kOfxParamPropHint, 0, d.hint);
    if (d.group) gProp->propSetString(p, kOfxParamPropParent, 0, d.group);
    gProp->propSetInt(p, kOfxParamPropAnimates, 0, d.animates ? 1 : 0);
    switch (d.type) {
        case kDbl:
            gProp->propSetDouble(p, kOfxParamPropDefault, 0, d.def);
            gProp->propSetDouble(p, kOfxParamPropMin, 0, d.lo);
            gProp->propSetDouble(p, kOfxParamPropMax, 0, d.hi);
            gProp->propSetDouble(p, kOfxParamPropDisplayMin, 0, d.dlo);
            gProp->propSetDouble(p, kOfxParamPropDisplayMax, 0, d.dhi);
            gProp->propSetDouble(p, kOfxParamPropIncrement, 0, 0.01);
            gProp->propSetInt(p, kOfxParamPropDigits, 0, 2);
            break;
        case kInt:
            gProp->propSetInt(p, kOfxParamPropDefault, 0, int(d.def));
            gProp->propSetInt(p, kOfxParamPropMin, 0, int(d.lo));
            gProp->propSetInt(p, kOfxParamPropMax, 0, int(d.hi));
            gProp->propSetInt(p, kOfxParamPropDisplayMin, 0, int(d.dlo));
            gProp->propSetInt(p, kOfxParamPropDisplayMax, 0, int(d.dhi));
            break;
        case kBool:
            gProp->propSetInt(p, kOfxParamPropDefault, 0, int(d.def));
            break;
        case kChoice:
            for (size_t i = 0; i < d.options.size(); ++i)
                gProp->propSetString(p, kOfxParamPropChoiceOption, int(i), d.options[i]);
            gProp->propSetInt(p, kOfxParamPropDefault, 0, int(d.def));
            break;
    }
}

OfxStatus describeInContext(OfxImageEffectHandle effect, OfxPropertySetHandle) {
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
    const char* openGroup = nullptr;
    for (const PDef& d : paramDefs()) {
        // Groups are defined just before their first child.
        if (d.group && (!openGroup || std::strcmp(openGroup, d.group) != 0)) {
            for (const GDef& g : kGroups)
                if (std::strcmp(g.name, d.group) == 0) {
                    OfxPropertySetHandle gp;
                    gParam->paramDefine(ps, kOfxParamTypeGroup, g.name, &gp);
                    gProp->propSetString(gp, kOfxPropLabel, 0, g.label);
                    gProp->propSetInt(gp, kOfxParamPropGroupOpen, 0, g.open ? 1 : 0);
                }
            openGroup = d.group;
        }
        defineParam(ps, d);
        if (d.id == P_MODE) {
            OfxPropertySetHandle p;
            gParam->paramDefine(ps, kOfxParamTypeChoice, kParamLook, &p);
            gProp->propSetString(p, kOfxPropLabel, 0, "Look");
            gProp->propSetString(p, kOfxParamPropHint, 0,
                                 "A starting point. Pick one and press Apply Look; then tweak.");
            for (size_t i = 0; i < looks().size(); ++i)
                gProp->propSetString(p, kOfxParamPropChoiceOption, int(i), looks()[i].name);
            gProp->propSetInt(p, kOfxParamPropDefault, 0, 0);
            gProp->propSetInt(p, kOfxParamPropAnimates, 0, 0);
            gProp->propSetInt(p, kOfxParamPropEvaluateOnChange, 0, 0);
            gParam->paramDefine(ps, kOfxParamTypePushButton, kParamApplyLook, &p);
            gProp->propSetString(p, kOfxPropLabel, 0, "Apply Look");
            gProp->propSetString(p, kOfxParamPropHint, 0,
                                 "Set the Motion, Time, Block and Colour controls to the Look.");
        }
        if (d.id == P_START) {
            OfxPropertySetHandle p;
            gParam->paramDefine(ps, kOfxParamTypePushButton, kParamSetStart, &p);
            gProp->propSetString(p, kOfxPropLabel, 0, "Start Mosh Here");
            gProp->propSetString(p, kOfxParamPropHint, 0, "Set Mosh Start Frame to the current frame.");
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
    for (const PDef& d : paramDefs()) gParam->paramGetHandle(ps, d.name, &in->h[d.id], nullptr);
    gParam->paramGetHandle(ps, kParamSetStart, &in->setStart, nullptr);
    gParam->paramGetHandle(ps, kParamLook, &in->look, nullptr);

    OfxPropertySetHandle props;
    gFx->getPropertySet(effect, &props);
    gProp->propSetPointer(props, kOfxPropInstanceData, 0, in);
    {
        Registry& r = registry();
        std::lock_guard<std::mutex> g(r.m);
        r.all.push_back(in);
    }
    return kOfxStatOK;
}

OfxStatus destroyInstance(OfxImageEffectHandle effect) {
    Instance* gone = getInstance(effect);
    {
        Registry& r = registry();
        std::lock_guard<std::mutex> g(r.m);
        r.all.erase(std::remove(r.all.begin(), r.all.end(), gone), r.all.end());
        if (r.all.empty()) r.scratch.clear();  // nothing left using the effect
    }
    delete gone;
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
        if (in) gParam->paramSetValue(in->h[P_START], int(std::lround(t)));
        return kOfxStatOK;
    }
    if (std::strcmp(name, kParamApplyLook) == 0) {
        Instance* in = getInstance(effect);
        if (!in || !in->look) return kOfxStatOK;
        int idx = 0;
        gParam->paramGetValue(in->look, &idx);
        if (idx < 0 || idx >= int(looks().size())) return kOfxStatOK;
        const Look& lk = looks()[size_t(idx)];
        OfxParamSetHandle ps;
        gFx->getParamSet(effect, &ps);
        gParam->paramEditBegin(ps, "Apply Look");
        for (const PDef& d : paramDefs()) {
            if (!d.group || !in->h[d.id]) continue;  // placement and Mix stay
            double v = d.def;
            for (auto& kv : lk.values)
                if (kv.first == d.id) v = kv.second;
            if (d.type == kDbl) gParam->paramSetValue(in->h[d.id], v);
            else gParam->paramSetValue(in->h[d.id], int(std::lround(v)));
        }
        gParam->paramEditEnd(ps);
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
    const int frame = int(std::lround(t));
    if (p.i(P_DIAG) != 0) return kOfxStatReplyDefault;
    if ((inMosh(p, frame) || armsMosh(p, frame)) && p.v[P_MIX] > 0.0) return kOfxStatReplyDefault;
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
    const int frame = int(std::lround(t));
    double range[2] = {t, t};
    if (inMosh(p, frame)) {
        const int S = effectiveStart(p, frame);
        double lo = std::min<double>(S - 1, t), hi = t;
        if (p.i(P_MODE) == kModeDrag)
            for (const mosh::StepSpec& s : mosh::schedule(S, frame, timeSettings(p)))
                if (s.move) {
                    lo = std::min<double>(lo, std::min(s.a, s.b));
                    hi = std::max<double>(hi, std::max(s.a, s.b));
                }
        range[0] = lo;
        range[1] = hi;
    }
    std::string prop = std::string("OfxImageClipPropFrameRange_") +
                       kOfxImageEffectSimpleSourceClipName;
    gProp->propSetDoubleN(outArgs, prop.c_str(), 2, range);
    return kOfxStatOK;
}

// 5x7 bitmap font: each glyph is 7 rows of 5 bits, top row first.
const uint8_t* glyph(char c) {
    struct G { char c; uint8_t r[7]; };
    static const G font[] = {
        {'0', {14, 17, 19, 21, 25, 17, 14}}, {'1', {4, 12, 4, 4, 4, 4, 14}},
        {'2', {14, 17, 1, 2, 4, 8, 31}},     {'3', {30, 1, 1, 14, 1, 1, 30}},
        {'4', {2, 6, 10, 18, 31, 2, 2}},     {'5', {31, 16, 30, 1, 1, 17, 14}},
        {'6', {6, 8, 16, 30, 17, 17, 14}},   {'7', {31, 1, 2, 4, 8, 8, 8}},
        {'8', {14, 17, 17, 14, 17, 17, 14}}, {'9', {14, 17, 17, 15, 1, 2, 12}},
        {'.', {0, 0, 0, 0, 0, 12, 12}},      {'-', {0, 0, 0, 31, 0, 0, 0}},
        {'+', {0, 4, 4, 31, 4, 4, 0}},       {'/', {1, 2, 2, 4, 8, 8, 16}},
        {':', {0, 12, 12, 0, 12, 12, 0}},    {'=', {0, 0, 31, 0, 31, 0, 0}},
        {'(', {2, 4, 8, 8, 8, 4, 2}},        {')', {8, 4, 2, 2, 2, 4, 8}},
        {'A', {14, 17, 17, 31, 17, 17, 17}}, {'B', {30, 17, 17, 30, 17, 17, 30}},
        {'C', {14, 17, 16, 16, 16, 17, 14}}, {'D', {30, 17, 17, 17, 17, 17, 30}},
        {'E', {31, 16, 16, 30, 16, 16, 31}}, {'F', {31, 16, 16, 30, 16, 16, 16}},
        {'G', {14, 17, 16, 23, 17, 17, 15}}, {'H', {17, 17, 17, 31, 17, 17, 17}},
        {'I', {14, 4, 4, 4, 4, 4, 14}},      {'K', {17, 18, 20, 24, 20, 18, 17}},
        {'L', {16, 16, 16, 16, 16, 16, 31}}, {'M', {17, 27, 21, 21, 17, 17, 17}},
        {'N', {17, 25, 21, 19, 17, 17, 17}}, {'O', {14, 17, 17, 17, 17, 17, 14}},
        {'P', {30, 17, 17, 30, 16, 16, 16}}, {'R', {30, 17, 17, 30, 20, 18, 17}},
        {'S', {15, 16, 16, 14, 1, 1, 30}},   {'T', {31, 4, 4, 4, 4, 4, 4}},
        {'U', {17, 17, 17, 17, 17, 17, 14}}, {'V', {17, 17, 17, 17, 17, 10, 4}},
        {'W', {17, 17, 17, 21, 21, 21, 10}}, {'X', {17, 17, 10, 4, 10, 17, 17}},
        {'Y', {17, 17, 10, 4, 4, 4, 4}},
    };
    for (const G& g : font)
        if (g.c == c) return g.r;
    return nullptr;
}

// Draws lines of text, top-left, on a bottom-up RGBA float image.
void drawText(float* data, int rowBytes, const OfxRectI& b, const std::vector<std::string>& lines) {
    const int w = b.x2 - b.x1, h = b.y2 - b.y1;
    const int sc = std::max(2, h / 300);
    const int lineH = 9 * sc, pad = 2 * sc;
    size_t longest = 0;
    for (auto& l : lines) longest = std::max(longest, l.size());
    const int boxW = std::min(w, int(longest) * 6 * sc + 2 * pad);
    const int boxH = std::min(h, int(lines.size()) * lineH + 2 * pad);
    auto px = [&](int x, int yTop) -> float* {  // yTop: rows from the top
        int y = h - 1 - yTop;
        if (x < 0 || x >= w || y < 0 || y >= h) return nullptr;
        return reinterpret_cast<float*>(reinterpret_cast<char*>(data) + std::ptrdiff_t(y) * rowBytes) + size_t(x) * 4;
    };
    for (int y = 0; y < boxH; ++y)
        for (int x = 0; x < boxW; ++x)
            if (float* p = px(x, y)) { p[0] *= 0.25f; p[1] *= 0.25f; p[2] *= 0.25f; p[3] = 1.f; }
    for (size_t li = 0; li < lines.size(); ++li)
        for (size_t ci = 0; ci < lines[li].size(); ++ci) {
            const uint8_t* g = glyph(char(std::toupper((unsigned char)lines[li][ci])));
            if (!g) continue;
            for (int gy = 0; gy < 7; ++gy)
                for (int gx = 0; gx < 5; ++gx) {
                    if (!(g[gy] & (16 >> gx))) continue;
                    for (int sy = 0; sy < sc; ++sy)
                        for (int sx = 0; sx < sc; ++sx)
                            if (float* p = px(pad + int(ci) * 6 * sc + gx * sc + sx, pad + int(li) * lineH + gy * sc + sy)) {
                                p[0] = p[1] = p[2] = 1.f;
                                p[3] = 1.f;
                            }
                }
        }
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
    // Released however this function ends, an exception (out of memory) included.
    struct Release {
        OfxPropertySetHandle img;
        ~Release() { gFx->clipReleaseImage(img); }
    } releaseDst{dstImg};
    void* dstData = nullptr;
    int dstRowBytes = 0;
    OfxRectI db;
    gProp->propGetPointer(dstImg, kOfxImagePropData, 0, &dstData);
    gProp->propGetInt(dstImg, kOfxImagePropRowBytes, 0, &dstRowBytes);
    gProp->propGetIntN(dstImg, kOfxImagePropBounds, 4, &db.x1);

    mosh::Image moshed;
    Frame real;
    bool haveMosh = false;
    BuildInfo info;
    {
        std::lock_guard<std::mutex> g(in->lock);
        if (inMosh(p, frame) && p.v[P_MIX] > 0.0) {
            try {
                haveMosh = buildMosh(effect, in, p, frame, rs[0] > 0 ? rs[0] : 1.0, moshed, real, info);
            } catch (const std::bad_alloc&) {
                // Out of memory: give everything back before reporting it.
                in->states.clear();
                in->motion.clear();
                in->kept.clear();
                in->bytes = 0;
                throw;
            }
            trimCaches(in);
        }
    }
    if (!haveMosh && !real.ok) fetchFrame(in->src, t, real);
    if (!real.ok && !haveMosh) {
        return gFx->abort(effect) ? kOfxStatOK : kOfxStatFailed;
    }

    const mosh::Image& m = haveMosh ? moshed : real.img;
    const OfxRectI sb = real.ok ? real.bounds : OfxRectI{db.x1, db.y1, db.x1 + m.w, db.y1 + m.h};
    const float mix = haveMosh ? float(p.v[P_MIX]) : 0.f;

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

    if (p.i(P_DIAG) != 0) {
        static const char* const src[] = {"MISSING", "FROM HOST", "REMEMBERED"};
        const int S = effectiveStart(p, frame);
        std::vector<std::string> lines;
        lines.push_back("ERIKSTV " + std::to_string(kVersionMajor) + "." + std::to_string(kVersionMinor) +
                        "  FRAME " + std::to_string(frame) + "  START " + std::to_string(S) + "  LENGTH " +
                        std::to_string(p.i(P_LENGTH)));
        if (info.tried) {
            lines.push_back(std::string("IN MOSH  RESULT: ") + (info.ok ? "MOSH" : "CLEAN (HELD FRAME MISSING)"));
            lines.push_back("HELD FRAME " + std::to_string(S - 1) + ": " + src[info.refSource]);
            if (info.from >= frame)
                lines.push_back("FIRST FRAME OF THE MOSH: SHOWS THE HELD FRAME");
            else
                lines.push_back("STEPS " + std::to_string(info.from + 1) + "-" + std::to_string(frame) + ": " +
                                std::to_string(info.applied) + " APPLIED  " + std::to_string(info.skipped) +
                                " SKIPPED" + (info.usedCache ? "  (CONTINUED)" : "  (FROM START)"));
            lines.push_back("OTHER FRAMES: " + std::to_string(info.fromHost) + " FROM HOST  " +
                            std::to_string(info.remembered) + " REMEMBERED  " + std::to_string(info.missing) +
                            " MISSING");
            if (info.gaveCurrent)
                lines.push_back("HOST GAVE THE CURRENT FRAME " + std::to_string(info.gaveCurrent) + " TIMES");
        } else if (armsMosh(p, frame)) {
            lines.push_back("FRAME BEFORE THE MOSH (REMEMBERED AS THE HELD FRAME)");
        } else {
            lines.push_back(p.v[P_MIX] > 0.0 ? "NOT IN MOSH" : "MIX IS 0");
        }
        drawText(static_cast<float*>(dstData), dstRowBytes, db, lines);
    }

    // Keep this frame: the next one's motion is measured against it, and the
    // frame before a mosh is the one the mosh holds.
    if (real.ok && p.v[P_MIX] > 0.0 && (inMosh(p, frame) || armsMosh(p, frame))) {
        std::lock_guard<std::mutex> g(in->lock);
        rememberFrame(in, p, frame, std::move(real));
        in->bytes = cachedBytes(in);
    }
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
                in->kept.clear();
                in->bytes = 0;
            }
            {
                Registry& r = registry();
                std::lock_guard<std::mutex> g(r.m);
                r.scratch.clear();
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
