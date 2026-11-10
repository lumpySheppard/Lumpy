// mosh_cli: run the datamosh engine over raw video, outside Resolve.
//
// Reads rgb24 frames on stdin, writes rgb24 frames on stdout, so ffmpeg can
// feed it and encode the result:
//
//   ffmpeg -i in.mov -f rawvideo -pix_fmt rgb24 - |
//     mosh_cli 1920 1080 --start 48 --length 60 |
//     ffmpeg -f rawvideo -pix_fmt rgb24 -s 1920x1080 -r 24 -i - out.mp4
//
// Handy for previewing settings quickly; the plugin uses the same engine.
#include <cstdio>
#include <cstdlib>
#include <cstring>
#include <string>
#include <vector>

#include "MoshEngine.h"

static void usage() {
    std::fprintf(stderr,
                 "usage: mosh_cli W H [--start N] [--length N] [--mode drag|bloom]\n"
                 "                [--block 8|16|32] [--range PX] [--scale F]\n"
                 "                [--residual F] [--heal F] [--smooth]\n");
    std::exit(2);
}

int main(int argc, char** argv) {
    if (argc < 3) usage();
    int w = std::atoi(argv[1]), h = std::atoi(argv[2]);
    if (w <= 0 || h <= 0) usage();
    int start = 0, length = 48;
    bool bloom = false;
    mosh::MotionSettings ms;
    mosh::StepSettings ss;
    for (int i = 3; i < argc; ++i) {
        std::string a = argv[i];
        auto next = [&]() -> const char* {
            if (i + 1 >= argc) usage();
            return argv[++i];
        };
        if (a == "--start") start = std::atoi(next());
        else if (a == "--length") length = std::atoi(next());
        else if (a == "--mode") bloom = std::string(next()) == "bloom";
        else if (a == "--block") ms.block = std::atoi(next());
        else if (a == "--range") ms.range = std::atoi(next());
        else if (a == "--scale") ss.motionScale = float(std::atof(next()));
        else if (a == "--residual") ss.residual = float(std::atof(next()));
        else if (a == "--heal") ss.heal = float(std::atof(next()));
        else if (a == "--smooth") ss.smooth = true;
        else usage();
    }

    const size_t frameBytes = size_t(w) * h * 3;
    std::vector<unsigned char> buf(frameBytes);
    mosh::Image cur(w, h), prev(w, h), state, a, b;
    mosh::MotionField bloomField;
    bool havePrev = false;

    for (int t = 0; std::fread(buf.data(), 1, frameBytes, stdin) == frameBytes; ++t) {
        for (size_t i = 0, n = size_t(w) * h; i < n; ++i) {
            cur.px[i * 4 + 0] = buf[i * 3 + 0] / 255.f;
            cur.px[i * 4 + 1] = buf[i * 3 + 1] / 255.f;
            cur.px[i * 4 + 2] = buf[i * 3 + 2] / 255.f;
            cur.px[i * 4 + 3] = 1.f;
        }

        const mosh::Image* out = &cur;
        if (t == start) {
            // Drag: this is the I-frame we "delete": keep showing the frame
            // before it. Bloom: freeze this frame and its motion.
            if (bloom) {
                state = cur;
                if (havePrev) bloomField = mosh::estimateMotion(cur, prev, ms);
            } else {
                state = havePrev ? prev : cur;
            }
            out = &state;
        } else if (t > start && t < start + length && !state.empty()) {
            if (bloom) {
                mosh::stepBloom(state, cur, bloomField, ss, a);
            } else {
                mosh::MotionField mf = mosh::estimateMotion(cur, prev, ms);
                mosh::stepDrag(state, cur, prev, mf, ss, a, b);
            }
            out = &state;
        }

        for (size_t i = 0, n = size_t(w) * h; i < n; ++i)
            for (int k = 0; k < 3; ++k) {
                float v = out->px[i * 4 + k];
                v = v < 0.f ? 0.f : (v > 1.f ? 1.f : v);
                buf[i * 3 + k] = (unsigned char)(v * 255.f + 0.5f);
            }
        std::fwrite(buf.data(), 1, frameBytes, stdout);
        std::swap(prev, cur);
        if (cur.empty()) cur = mosh::Image(w, h);
        havePrev = true;
    }
    return 0;
}
