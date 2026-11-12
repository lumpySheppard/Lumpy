// gate_cli: preview the gate flare outside Resolve on raw rgb24 video.
//
//   ffmpeg -i in.mov -f rawvideo -pix_fmt rgb24 - |
//     gate_cli 1920 1080 --crop 6 --intensity 1 |
//     ffmpeg -f rawvideo -pix_fmt rgb24 -s 1920x1080 -r 24 -i - -pix_fmt yuv420p out.mp4
#include <cmath>
#include <cstdio>
#include <cstdlib>
#include <map>
#include <string>
#include <vector>

#include "GateEngine.h"

static void usage() {
    std::fprintf(stderr,
                 "usage: gate_cli W H [--crop PCT] [--threshold F] [--knee F] [--intensity F] [--reach PCT]\n"
                 "                [--depth PCT] [--spread PCT] [--fan F] [--edge-glow F] [--source-colour F]\n"
                 "                [--encoding N] [--show-gate 1] [--no-crop 1]\n");
    std::exit(2);
}

int main(int argc, char** argv) {
    if (argc < 3) usage();
    const int w = std::atoi(argv[1]), h = std::atoi(argv[2]);
    if (w <= 0 || h <= 0) usage();
    gate::Settings s;
    s.upSign = -1;  // rgb24 rows are top-down
    float showGate = 0, noCrop = 0, enc = 0, shapeF = 0, compF = 0;
    std::map<std::string, float*> f = {
        {"--crop", &s.crop}, {"--threshold", &s.threshold}, {"--knee", &s.knee}, {"--isolate", &s.isolate}, {"--streaks", &s.streaks}, {"--streak-detail", &s.streakDetail},
        {"--shimmer", &s.shimmer}, {"--shimmer-speed", &s.shimmerSpeed}, {"--core", &s.core},
        {"--fringe", &s.fringe}, {"--blade-angle", &s.bladeAngle}, {"--blade-softness", &s.bladeSoftness},
        {"--shape", &shapeF}, {"--composite", &compF}, {"--light-size", &s.lightSize},
        {"--intensity", &s.intensity}, {"--reach", &s.reach}, {"--depth", &s.depth},
        {"--spread", &s.spread}, {"--fan", &s.fan}, {"--edge-glow", &s.edgeGlow},
        {"--source-colour", &s.sourceColour}, {"--show-gate", &showGate}, {"--no-crop", &noCrop},
        {"--encoding", &enc}};
    for (int i = 3; i < argc; ++i) {
        std::string a = argv[i];
        if (i + 1 >= argc || !f.count(a)) usage();
        *f[a] = float(std::atof(argv[++i]));
    }
    s.showGate = showGate != 0;
    s.cropOutput = noCrop == 0;
    s.encoding = int(enc);
    s.shape = int(shapeF);
    s.composite = int(compF);
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
