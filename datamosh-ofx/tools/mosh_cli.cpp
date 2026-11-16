// mosh_cli: run the datamosh engine over raw video, outside Resolve.
//
// Reads rgb24 frames on stdin, writes rgb24 frames on stdout, so ffmpeg can
// feed it and encode the result:
//
//   ffmpeg -i in.mov -f rawvideo -pix_fmt rgb24 - |
//     mosh_cli 1920 1080 --start 48 --length 60 |
//     ffmpeg -f rawvideo -pix_fmt rgb24 -s 1920x1080 -r 24 -i - out.mp4
//
// The whole clip is held in memory (time glitches can jump around), so keep
// previews short. Handy for trying settings; the plugin uses the same engine.
#include <cstdio>
#include <cstdlib>
#include <cstring>
#include <map>
#include <string>
#include <vector>

#include "MoshEngine.h"

static void usage() {
    std::fprintf(stderr,
                 "usage: mosh_cli W H [options] < in.rgb > out.rgb\n"
                 "  --start N --length N --cycle N --mode drag|bloom --seed N\n"
                 "  motion: --block 4..64 --variety F --range PX --scale F --residual F\n"
                 "          --heal F --smooth\n"
                 "  time:   --repeat F --freeze F --jump-back F --jump-fwd F --reverse F\n"
                 "          --event-len N --jump-dist N --every N\n"
                 "  blocks: --stuck F --scatter F --streak F --flatten F --hold N\n"
                 "  colour: --chroma-drift F --bleed F --sat-build F --hue-drift DEG\n"
                 "          --colour-err F --colour-err-strength F\n");
    std::exit(2);
}

int main(int argc, char** argv) {
    if (argc < 3) usage();
    const int w = std::atoi(argv[1]), h = std::atoi(argv[2]);
    if (w <= 0 || h <= 0) usage();
    int start = 0, length = 48, cycle = 0, seed = 0;
    bool bloom = false;
    mosh::MotionSettings ms;
    mosh::StepSettings ss;
    mosh::TimeSettings ts;
    ss.glitch.upSign = -1;  // rgb24 rows are top-down

    std::map<std::string, float*> f = {
        {"--variety", &ms.variety}, {"--scale", &ss.motionScale}, {"--residual", &ss.residual},
        {"--heal", &ss.heal}, {"--repeat", &ts.repeat}, {"--freeze", &ts.freeze},
        {"--jump-back", &ts.jumpBack}, {"--jump-fwd", &ts.jumpFwd}, {"--reverse", &ts.reverse},
        {"--stuck", &ss.glitch.stuck}, {"--scatter", &ss.glitch.scatter},
        {"--streak", &ss.glitch.streak}, {"--flatten", &ss.glitch.flatten},
        {"--colour-err", &ss.glitch.colourErr},
        {"--colour-err-strength", &ss.glitch.colourErrStrength},
        {"--chroma-drift", &ss.colour.chromaDrift}, {"--bleed", &ss.colour.bleed},
        {"--sat-build", &ss.colour.satBuild}, {"--hue-drift", &ss.colour.hueDrift}};
    std::map<std::string, int*> n = {
        {"--start", &start}, {"--length", &length}, {"--cycle", &cycle}, {"--seed", &seed},
        {"--block", &ms.block}, {"--range", &ms.range}, {"--event-len", &ts.maxLen},
        {"--jump-dist", &ts.maxJump}, {"--every", &ts.every}, {"--hold", &ss.glitch.hold}};
    for (int i = 3; i < argc; ++i) {
        std::string a = argv[i];
        if (a == "--smooth") { ss.smooth = true; continue; }
        if (i + 1 >= argc) usage();
        if (a == "--mode") { bloom = std::string(argv[++i]) == "bloom"; continue; }
        if (f.count(a)) { *f[a] = float(std::atof(argv[++i])); continue; }
        if (n.count(a)) { *n[a] = std::atoi(argv[++i]); continue; }
        usage();
    }
    ms.seed = ss.seed = ts.seed = uint32_t(seed);
    ss.range = ms.range;

    const size_t fb = size_t(w) * h * 3;
    std::vector<std::vector<unsigned char>> frames;
    for (std::vector<unsigned char> buf(fb); std::fread(buf.data(), 1, fb, stdin) == fb;)
        frames.push_back(buf);
    const int nf = int(frames.size());

    auto toImage = [&](int t, mosh::Image& img) {
        img = mosh::Image(w, h);
        const unsigned char* s = frames[t].data();
        for (size_t i = 0, np = size_t(w) * h; i < np; ++i) {
            img.px[i * 4 + 0] = s[i * 3 + 0] / 255.f;
            img.px[i * 4 + 1] = s[i * 3 + 1] / 255.f;
            img.px[i * 4 + 2] = s[i * 3 + 2] / 255.f;
            img.px[i * 4 + 3] = 1.f;
        }
    };
    std::map<int, mosh::Image> cache;
    auto get = [&](int t) -> const mosh::Image* {
        if (t < 0 || t >= nf) return nullptr;
        auto it = cache.find(t);
        if (it != cache.end()) return &it->second;
        if (cache.size() > 6) cache.erase(cache.begin());
        toImage(t, cache[t]);
        return &cache[t];
    };

    mosh::Image state;
    mosh::Scratch scratch;
    mosh::MotionField bloomField;
    int curStart = -1;
    std::vector<mosh::StepSpec> plan;
    std::vector<unsigned char> out(fb);

    for (int t = 0; t < nf; ++t) {
        int S = start;
        if (cycle > 0 && t >= start) S = start + (t - start) / cycle * cycle;
        const bool in = t >= S && t < S + length;
        mosh::Image real;
        const mosh::Image* show = &real;
        if (in) {
            if (S != curStart) {  // new mosh window
                curStart = S;
                if (bloom) {
                    state = *get(S);
                    const mosh::Image* b = get(S - 1);
                    bloomField = b ? mosh::estimateMotion(state, *b, ms) : mosh::MotionField{};
                } else {
                    state = get(S - 1) ? *get(S - 1) : *get(S);
                    plan = mosh::schedule(S, S + length, ts);
                }
            } else if (bloom) {
                mosh::step(state, nullptr, nullptr, get(t), bloomField, ss, t, scratch);
            } else {
                const mosh::StepSpec& sp = plan[t - S - 1];
                const mosh::Image* A = sp.move ? get(sp.a) : nullptr;
                mosh::Image a = A ? *A : mosh::Image();  // copy: the cache may evict
                const mosh::Image* B = A ? get(sp.b) : nullptr;
                if (A && B) {
                    mosh::Image b = *B;
                    mosh::MotionField mf = mosh::estimateMotion(a, b, ms);
                    mosh::step(state, &a, &b, get(t), mf, ss, t, scratch);
                }
            }
            show = &state;
        } else {
            real = *get(t);
        }
        for (size_t i = 0, np = size_t(w) * h; i < np; ++i)
            for (int k = 0; k < 3; ++k) {
                float v = show->px[i * 4 + k];
                v = v < 0.f ? 0.f : (v > 1.f ? 1.f : v);
                out[i * 3 + k] = (unsigned char)(v * 255.f + 0.5f);
            }
        std::fwrite(out.data(), 1, fb, stdout);
    }
    return 0;
}
