// flash_cli: preview the film flash outside Resolve.
//
// Reads rgb24 frames (a clip that already contains the cut) on stdin and
// writes rgb24 on stdout. The transition is centred on --cut and lasts
// --length frames, like a transition dropped on that cut in Resolve.
//
//   ffmpeg -i in.mov -f rawvideo -pix_fmt rgb24 - |
//     flash_cli 1920 1080 --cut 48 --length 24 |
//     ffmpeg -f rawvideo -pix_fmt rgb24 -s 1920x1080 -r 24 -i - out.mp4
//
// --stops F instead applies a constant flash to every frame (the effect mode).
#include <cmath>
#include <cstdio>
#include <cstdlib>
#include <map>
#include <string>
#include <vector>

#include "FlashEngine.h"

static void usage() {
    std::fprintf(stderr,
                 "usage: flash_cli W H [--cut N --length N | --stops F]\n"
                 "  envelope: --peak F --cut-point F --hold F --run-out F --run-up F --flicker F --seed N\n"
                 "  film:     --response F --colour-shift F --fog F --halation F --halation-radius PX\n"
                 "            --unevenness F --angle DEG --leak F\n");
    std::exit(2);
}

int main(int argc, char** argv) {
    if (argc < 3) usage();
    const int w = std::atoi(argv[1]), h = std::atoi(argv[2]);
    if (w <= 0 || h <= 0) usage();
    int cut = 24, length = 24, seed = 1;
    float constStops = -1.f;
    flash::Envelope env;
    flash::Look look;
    look.upSign = -1;  // rgb24 rows are top-down
    std::map<std::string, float*> f = {
        {"--peak", &env.peak}, {"--cut-point", &env.cut}, {"--hold", &env.hold},
        {"--run-out", &env.runOut}, {"--run-up", &env.runUp}, {"--flicker", &env.flicker},
        {"--response", &look.response}, {"--colour-shift", &look.colourShift},
        {"--fog", &look.fog}, {"--halation", &look.halation},
        {"--halation-radius", &look.halationRadius}, {"--unevenness", &look.unevenness},
        {"--angle", &look.angle}, {"--leak", &look.leak}, {"--stops", &constStops}};
    std::map<std::string, int*> n = {{"--cut", &cut}, {"--length", &length}, {"--seed", &seed}};
    for (int i = 3; i < argc; ++i) {
        std::string a = argv[i];
        if (i + 1 >= argc) usage();
        if (f.count(a)) { *f[a] = float(std::atof(argv[++i])); continue; }
        if (n.count(a)) { *n[a] = std::atoi(argv[++i]); continue; }
        usage();
    }
    env.seed = uint32_t(seed);
    look.halationRadius *= w / 1920.f;  // the default is for HD

    const size_t fb = size_t(w) * h * 3;
    std::vector<unsigned char> buf(fb);
    flash::Image img(w, h);
    const int start = cut - length / 2;
    for (int t = 0; std::fread(buf.data(), 1, fb, stdin) == fb; ++t) {
        for (size_t i = 0, np = size_t(w) * h; i < np; ++i) {
            for (int k = 0; k < 3; ++k) img.px[i * 4 + k] = buf[i * 3 + k] / 255.f;
            img.px[i * 4 + 3] = 1.f;
        }
        float stops = 0.f;
        if (constStops >= 0.f) {
            stops = constStops;
        } else if (t >= start && t < start + length) {
            float p = length > 1 ? float(t - start) / float(length - 1) : 0.5f;
            stops = flash::envelopeStops(env, p, t);
        }
        flash::render(img, stops, look, img);
        for (size_t i = 0, np = size_t(w) * h; i < np; ++i)
            for (int k = 0; k < 3; ++k) {
                float v = std::min(std::max(img.px[i * 4 + k], 0.f), 1.f);
                buf[i * 3 + k] = (unsigned char)(v * 255.f + 0.5f);
            }
        std::fwrite(buf.data(), 1, fb, stdout);
    }
    return 0;
}
