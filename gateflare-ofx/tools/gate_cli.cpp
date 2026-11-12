// gate_cli: preview the gate flare outside Resolve on raw rgb24 video.
//
//   ffmpeg -i in.mov -f rawvideo -pix_fmt rgb24 - |
//     gate_cli 1920 1080 --look-top 1 --look-side 5 |
//     ffmpeg -f rawvideo -pix_fmt rgb24 -s 1920x1080 -r 24 -i - -pix_fmt yuv420p out.mp4
//
// Looks: 0 Custom (the custom flags below), 1 Soft Haze, 2 Thin Streak,
// 3 Cool Column, 4 Blue Blade, 5 Side Blast.
#include <algorithm>
#include <cmath>
#include <cstdio>
#include <cstdlib>
#include <map>
#include <string>
#include <vector>

#include "GateEngine.h"

static void usage() {
    std::fprintf(stderr,
                 "usage: gate_cli W H [options]\n"
                 "  edges: --look-top N --look-top-b N --blend-top F --look-side N --look-side-b N --blend-side F\n"
                 "  custom look: --shape N --intensity F --flare-start PCT --reach PCT --depth PCT --spread PCT\n"
                 "               --fan F --edge-glow F --blade-angle DEG --blade-softness PCT --streaks F\n"
                 "               --streak-detail F --shimmer F --shimmer-speed F --core F --fringe F --source-colour F\n"
                 "               --pinch PCT --lean DEG --tint-r/g/b F --far-r/g/b F\n"
                 "               --curve F --veil F --iridescence F --bands F --along F\n"
                 "  sources: --threshold F --knee F --isolate F --light-size PCT\n"
                 "  other: --crop PCT --no-crop 1 --show-gate 1 --encoding N --composite N --seed N\n");
    std::exit(2);
}

int main(int argc, char** argv) {
    if (argc < 3) usage();
    const int w = std::atoi(argv[1]), h = std::atoi(argv[2]);
    if (w <= 0 || h <= 0) usage();
    gate::Settings s;
    s.upSign = -1;  // rgb24 rows are top-down
    gate::Look c;
    float showGate = 0, noCrop = 0, enc = 0, comp = 0, shape = 0, seed = 1;
    float lookTop = 0, lookTopB = 1, blendTop = 0, lookSide = 0, lookSideB = 1, blendSide = 0;
    float focusTop = 0, focusSide = 0;
    std::map<std::string, float*> f = {
        {"--crop", &s.crop}, {"--threshold", &s.threshold}, {"--knee", &s.knee}, {"--isolate", &s.isolate},
        {"--light-size", &s.lightSize}, {"--intensity", &c.intensity}, {"--flare-start", &c.flareStart},
        {"--reach", &c.reach}, {"--onset", &c.onset}, {"--wait-exit", &c.waitExit}, {"--depth", &c.depth}, {"--spread", &c.spread}, {"--fan", &c.fan},
        {"--edge-glow", &c.edgeGlow}, {"--blade-angle", &c.bladeAngle}, {"--blade-softness", &c.bladeSoftness},
        {"--streaks", &c.streaks}, {"--streak-detail", &c.streakDetail}, {"--shimmer", &c.shimmer},
        {"--shimmer-speed", &c.shimmerSpeed}, {"--core", &c.core}, {"--fringe", &c.fringe},
        {"--source-colour", &c.sourceColour}, {"--saturation", &c.saturation}, {"--pinch", &c.pinch}, {"--lean", &c.lean},
        {"--aim", &c.aim}, {"--evolve", &c.evolve}, {"--hue-drift", &c.hueDrift},
        {"--falloff", &c.falloff}, {"--curve", &c.curve}, {"--veil", &c.veil}, {"--iridescence", &c.iridescence}, {"--bands", &c.rainbowBands},
        {"--along", &c.rainbowAlong}, {"--edges", &c.prismEdges},
        {"--tint-r", &c.tint[0]}, {"--tint-g", &c.tint[1]}, {"--tint-b", &c.tint[2]},
        {"--far-r", &c.farTint[0]}, {"--far-g", &c.farTint[1]}, {"--far-b", &c.farTint[2]},
        {"--exit-r", &c.exitTint[0]}, {"--exit-g", &c.exitTint[1]}, {"--exit-b", &c.exitTint[2]}, {"--show-gate", &showGate}, {"--no-crop", &noCrop},
        {"--encoding", &enc}, {"--composite", &comp}, {"--shape", &shape}, {"--seed", &seed},
        {"--look-top", &lookTop}, {"--look-top-b", &lookTopB}, {"--blend-top", &blendTop},
        {"--look-side", &lookSide}, {"--look-side-b", &lookSideB}, {"--blend-side", &blendSide},
        {"--focus-top", &focusTop}, {"--focus-side", &focusSide}};
    for (int i = 3; i < argc; ++i) {
        std::string a = argv[i];
        if (i + 1 >= argc || !f.count(a)) usage();
        *f[a] = float(std::atof(argv[++i]));
    }
    c.shape = int(shape);
    s.showGate = showGate != 0;
    s.cropOutput = noCrop == 0;
    s.encoding = int(enc);
    s.composite = int(comp);
    s.seed = unsigned(seed);
    s.pair[0].a = gate::presetLook(int(lookTop), c);
    s.pair[0].b = gate::presetLook(int(lookTopB), c);
    s.pair[0].blend = blendTop;
    s.pair[1].a = gate::presetLook(int(lookSide), c);
    s.pair[1].b = gate::presetLook(int(lookSideB), c);
    s.pair[1].blend = blendSide;
    for (gate::Look* l : {&s.pair[0].a, &s.pair[0].b}) l->pinch = std::max(0.f, l->pinch + focusTop);
    for (gate::Look* l : {&s.pair[1].a, &s.pair[1].b}) l->pinch = std::max(0.f, l->pinch + focusSide);
    const size_t fb = size_t(w) * h * 3;
    std::vector<unsigned char> buf(fb);
    gate::Image img(w, h), out;
    for (int frame = 0; std::fread(buf.data(), 1, fb, stdin) == fb; ++frame) {
        s.time = float(frame);
        for (size_t i = 0, n = size_t(w) * h; i < n; ++i) {
            for (int k = 0; k < 3; ++k) img.px[i * 4 + k] = buf[i * 3 + k] / 255.f;
            img.px[i * 4 + 3] = 1.f;
        }
        gate::render(img, s, out);
        for (size_t i = 0, n = size_t(w) * h; i < n; ++i)
            for (int k = 0; k < 3; ++k) {
                float v = std::min(std::max(out.px[i * 4 + k], 0.f), 1.f);
                buf[i * 3 + k] = (unsigned char)(v * 255.f + 0.5f);
            }
        std::fwrite(buf.data(), 1, fb, stdout);
    }
    return 0;
}
