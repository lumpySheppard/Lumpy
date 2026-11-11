// encoding_check: sanity-checks the log curves (18% grey code values from the
// manufacturers' documentation, plus decode/encode round trips).
#include <cmath>
#include <cstdio>
#include <string>

#include "FlashEngine.h"

int main() {
    using namespace flash;
    struct Ref { int enc; const char* name; float grey; };  // grey < 0: round trip only
    const Ref refs[] = {
        {kLogC3, "ARRI LogC3", 0.391007f},     {kLogC4, "ARRI LogC4", 0.278396f},
        {kSLog3, "Sony S-Log3", 420.f / 1023.f}, {kVLog, "Panasonic V-Log", 0.423311f},
        {kLog3G10, "RED Log3G10", 1.f / 3.f},  {kAppleLog, "Apple Log", -1.f},
        {kBMDFilmGen5, "Blackmagic Film Gen 5", -1.f}, {kDaVinciIntermediate, "DaVinci Intermediate", 0.336043f},
        {kCineon, "Cineon", -1.f},
    };
    int bad = 0;
    for (const Ref& r : refs) {
        float g = fromLinear(0.18f, r.enc);
        bool ok = r.grey < 0 || std::fabs(g - r.grey) < 0.002f;
        float worst = 0;
        for (float l : {0.0f, 0.001f, 0.005f, 0.01f, 0.05f, 0.18f, 0.5f, 1.f, 4.f, 16.f, 50.f}) {
            float back = toLinear(fromLinear(l, r.enc), r.enc);
            worst = std::fmax(worst, std::fabs(back - l) / std::fmax(l, 0.01f));
        }
        ok = ok && worst < 1e-3f;
        std::printf("%-22s grey %.4f%s  round-trip error %.2g  %s\n", r.name, g,
                    r.grey < 0 ? "" : (std::string(" (want ") + std::to_string(r.grey).substr(0, 6) + ")").c_str(),
                    worst, ok ? "ok" : "FAIL");
        bad += !ok;
    }
    return bad ? 1 : 0;
}
