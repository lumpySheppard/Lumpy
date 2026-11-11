// Encodings.h: colour encodings <-> linear light, shared by the OFX plugins.
// Header-only. Curves follow the manufacturers' published formulas; see
// filmflash-ofx/tools/encoding_check.cpp for the checks.
#pragma once

#include <algorithm>
#include <cmath>

namespace encodings {

enum Encoding {
    kRec709 = 0,      // display, gamma 2.4
    kSRGB = 1,
    kGamma22 = 2,
    kLinearDisplay = 3,
    kLinearScene = 4,
    kDaVinciIntermediate = 5,
    kCineon = 6,
    kLogC3 = 7,
    kLogC4 = 8,
    kSLog3 = 9,
    kVLog = 10,
    kLog3G10 = 11,
    kAppleLog = 12,
    kBMDFilmGen5 = 13,
    kEncodingCount = 14,
};

inline bool isSceneReferred(int enc) { return enc >= kLinearScene; }

// UI names, in enum order.
inline const char* const* names() {
    static const char* const n[] = {"Rec.709 / Gamma 2.4", "sRGB", "Gamma 2.2", "Linear (display)",
                                    "Linear (scene)", "DaVinci Intermediate", "Cineon Log", "ARRI LogC3",
                                    "ARRI LogC4", "Sony S-Log3", "Panasonic V-Log", "RED Log3G10",
                                    "Apple Log", "Blackmagic Film Gen 5"};
    return n;
}


inline const float kCineonBlack = 0.0107977516f;  // 10^((95 - 685) / 300)

// DaVinci Intermediate constants (Blackmagic's published formula).
inline const float kDiA = 0.0075f, kDiB = 7.0f, kDiC = 0.07329248f, kDiM = 10.44426855f;
inline const float kDiLinCut = 0.00262409f, kDiLogCut = 0.02740668f;

// Camera log curves, from the manufacturers' published formulas.
namespace logc3 {
inline const float cut = 0.010591f, a = 5.555556f, b = 0.052272f, c = 0.247190f, d = 0.385537f,
            e = 5.367655f, f = 0.092809f;
}
namespace logc4 {
inline const float a = (262144.f - 16.f) / 117.45f, b = (1023.f - 95.f) / 1023.f, c = 95.f / 1023.f;
inline const float s = (7.f * 0.69314718f * std::exp2(7.f - 14.f * c / b)) / (a * b);
inline const float t = (std::exp2(14.f * (-c / b) + 6.f) - 64.f) / a;
}
namespace vlog {
inline const float cut1 = 0.01f, cut2 = 0.181f, b = 0.00873f, c = 0.241514f, d = 0.598206f;
}
namespace log3g10 {
inline const float a = 0.224282f, b = 155.975327f, c = 0.01f, g = 15.1927f;
}
namespace applelog {
inline const float R0 = -0.05641088f, Rt = 0.01f, c = 47.28711236f, beta = 0.00964052f,
            gamma = 0.08550479f, delta = 0.69336945f;
inline const float Pt = c * (Rt - R0) * (Rt - R0);
}
namespace bmd5 {
inline const float A = 0.08692876065491224f, B = 0.005494072432257808f, C = 0.5300133392291939f,
            D = 8.283605932402494f, E = 0.09246575342465753f, LIN_CUT = 0.005f;
inline const float LOG_CUT = D * LIN_CUT + E;
}

inline float decode(float v, int enc) {
    switch (enc) {
        case kLogC3:
            return v > logc3::e * logc3::cut + logc3::f
                       ? (std::pow(10.f, (v - logc3::d) / logc3::c) - logc3::b) / logc3::a
                       : (v - logc3::f) / logc3::e;
        case kLogC4:
            return v >= 0.f ? (std::exp2(14.f * (v - logc4::c) / logc4::b + 6.f) - 64.f) / logc4::a
                            : v * logc4::s + logc4::t;
        case kSLog3:
            return v >= 171.2102946929f / 1023.f
                       ? std::pow(10.f, (v * 1023.f - 420.f) / 261.5f) * 0.19f - 0.01f
                       : (v * 1023.f - 95.f) * 0.01125f / (171.2102946929f - 95.f);
        case kVLog:
            return v < vlog::cut2 ? (v - 0.125f) / 5.6f : std::pow(10.f, (v - vlog::d) / vlog::c) - vlog::b;
        case kLog3G10:
            return v < 0.f ? v / log3g10::g - log3g10::c
                           : (std::pow(10.f, v / log3g10::a) - 1.f) / log3g10::b - log3g10::c;
        case kAppleLog:
            if (v >= applelog::Pt) return std::exp2((v - applelog::delta) / applelog::gamma) - applelog::beta;
            return std::sqrt(std::max(v, 0.f) / applelog::c) + applelog::R0;
        case kBMDFilmGen5:
            return v < bmd5::LOG_CUT ? (v - bmd5::E) / bmd5::D : std::exp((v - bmd5::C) / bmd5::A) - bmd5::B;
        case kRec709: return std::pow(std::max(v, 0.f), 2.4f);
        case kSRGB:
            v = std::max(v, 0.f);
            return v <= 0.04045f ? v / 12.92f : std::pow((v + 0.055f) / 1.055f, 2.4f);
        case kGamma22: return std::pow(std::max(v, 0.f), 2.2f);
        case kDaVinciIntermediate:
            return v <= kDiLogCut ? v / kDiM : std::exp2(v / kDiC - kDiB) - kDiA;
        case kCineon:
            return (std::pow(10.f, (v * 1023.f - 685.f) / 300.f) - kCineonBlack) / (1.f - kCineonBlack);
        default: return v;  // linear
    }
}

inline float encode(float l, int enc) {
    switch (enc) {
        case kLogC3:
            return l > logc3::cut ? logc3::c * std::log10(logc3::a * l + logc3::b) + logc3::d
                                  : logc3::e * l + logc3::f;
        case kLogC4:
            return l >= logc4::t ? (std::log2(logc4::a * l + 64.f) - 6.f) / 14.f * logc4::b + logc4::c
                                 : (l - logc4::t) / logc4::s;
        case kSLog3:
            return l >= 0.01125f ? (420.f + std::log10((l + 0.01f) / 0.19f) * 261.5f) / 1023.f
                                 : (l * (171.2102946929f - 95.f) / 0.01125f + 95.f) / 1023.f;
        case kVLog:
            return l < vlog::cut1 ? 5.6f * l + 0.125f : vlog::c * std::log10(l + vlog::b) + vlog::d;
        case kLog3G10: {
            float x = l + log3g10::c;
            return x < 0.f ? x * log3g10::g : log3g10::a * std::log10(x * log3g10::b + 1.f);
        }
        case kAppleLog:
            if (l >= applelog::Rt) return applelog::gamma * std::log2(l + applelog::beta) + applelog::delta;
            if (l >= applelog::R0) return applelog::c * (l - applelog::R0) * (l - applelog::R0);
            return 0.f;
        case kBMDFilmGen5:
            return l < bmd5::LIN_CUT ? bmd5::D * l + bmd5::E : bmd5::A * std::log(l + bmd5::B) + bmd5::C;
        case kRec709: return std::pow(std::max(l, 0.f), 1.f / 2.4f);
        case kSRGB:
            l = std::max(l, 0.f);
            return l <= 0.0031308f ? l * 12.92f : 1.055f * std::pow(l, 1.f / 2.4f) - 0.055f;
        case kGamma22: return std::pow(std::max(l, 0.f), 1.f / 2.2f);
        case kDaVinciIntermediate:
            return l <= kDiLinCut ? l * kDiM : (std::log2(l + kDiA) + kDiB) * kDiC;
        case kCineon: {
            float x = std::max(l * (1.f - kCineonBlack) + kCineonBlack, 1e-10f);
            return (685.f + 300.f * std::log10(x)) / 1023.f;
        }
        default: return l;
    }
}


}  // namespace encodings
