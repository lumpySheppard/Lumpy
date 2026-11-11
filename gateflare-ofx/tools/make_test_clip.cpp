// make_test_clip: a dark, textured 48-frame rgb24 clip with a bright bar that
// drifts up and out through the top edge (for the GateFlare tests).
#include <cmath>
#include <cstdio>
#include <cstdlib>
#include <vector>

int main(int argc, char** argv) {
    if (argc < 4) { std::fprintf(stderr, "usage: make_test_clip W H OUT.rgb\n"); return 2; }
    const int w = std::atoi(argv[1]), h = std::atoi(argv[2]);
    FILE* f = std::fopen(argv[3], "wb");
    if (!f) return 1;
    std::vector<unsigned char> buf(size_t(w) * h * 3);
    for (int t = 0; t < 48; ++t) {
        const float barY = h * 0.3f - t * h * 0.01f;  // top-down; leaves the frame around t = 30
        for (int y = 0; y < h; ++y)
            for (int x = 0; x < w; ++x) {
                unsigned char* p = &buf[(size_t(y) * w + x) * 3];
                float v = 0.12f + 0.04f * std::sin(x * 0.3f) * std::cos(y * 0.2f);
                const bool bar = std::fabs(y - barY) < h * 0.02f && std::fabs(x - w * 0.5f) < w * 0.12f;
                for (int c = 0; c < 3; ++c) p[c] = (unsigned char)(255.f * (bar ? (c == 2 ? 0.9f : 1.f) : v));
            }
        std::fwrite(buf.data(), 1, buf.size(), f);
    }
    std::fclose(f);
    return 0;
}
