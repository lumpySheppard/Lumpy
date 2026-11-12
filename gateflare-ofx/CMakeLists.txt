cmake_minimum_required(VERSION 3.16)

# Universal (Apple silicon + Intel) by default on macOS. Must be set before project().
if(APPLE AND NOT CMAKE_OSX_ARCHITECTURES)
  set(CMAKE_OSX_ARCHITECTURES "arm64;x86_64" CACHE STRING "" FORCE)
endif()
set(CMAKE_OSX_DEPLOYMENT_TARGET "11.0" CACHE STRING "Minimum macOS")

project(GateFlare VERSION 1.10.0 LANGUAGES CXX)

set(CMAKE_CXX_STANDARD 17)
set(CMAKE_CXX_STANDARD_REQUIRED ON)
if(NOT CMAKE_BUILD_TYPE)
  set(CMAKE_BUILD_TYPE Release)
endif()

find_package(Threads REQUIRED)
set(OFX_INCLUDE ${CMAKE_CURRENT_SOURCE_DIR}/../third_party/openfx/include)

add_library(gate_engine STATIC src/GateEngine.cpp)
target_include_directories(gate_engine PUBLIC src ${CMAKE_CURRENT_SOURCE_DIR}/../shared)
set_target_properties(gate_engine PROPERTIES POSITION_INDEPENDENT_CODE ON CXX_VISIBILITY_PRESET hidden)
target_link_libraries(gate_engine PUBLIC Threads::Threads)

if(APPLE)
  set(OFX_ARCH_DIR MacOS)
elseif(WIN32)
  set(OFX_ARCH_DIR Win64)
else()
  set(OFX_ARCH_DIR Linux-x86-64)
endif()
set(BUNDLE_DIR ${CMAKE_BINARY_DIR}/GateFlare.ofx.bundle)

add_library(GateFlare MODULE src/GateFlarePlugin.cpp)
target_include_directories(GateFlare PRIVATE ${OFX_INCLUDE})
target_link_libraries(GateFlare PRIVATE gate_engine)
set_target_properties(GateFlare PROPERTIES
  PREFIX "" SUFFIX ".ofx"
  CXX_VISIBILITY_PRESET hidden
  VISIBILITY_INLINES_HIDDEN ON
  LIBRARY_OUTPUT_DIRECTORY ${BUNDLE_DIR}/Contents/${OFX_ARCH_DIR})
if(NOT MSVC)
  target_compile_options(GateFlare PRIVATE -Wall -Wextra)
  target_compile_options(gate_engine PRIVATE -Wall -Wextra)
endif()
if(APPLE)
  configure_file(resources/Info.plist.in ${BUNDLE_DIR}/Contents/Info.plist @ONLY)
endif()

add_executable(gate_cli tools/gate_cli.cpp)
target_link_libraries(gate_cli PRIVATE gate_engine)
add_executable(make_test_clip tools/make_test_clip.cpp)

if(NOT WIN32)
  add_executable(minihost ${CMAKE_CURRENT_SOURCE_DIR}/../ofx-testhost/minihost.cpp)
  target_include_directories(minihost PRIVATE ${OFX_INCLUDE})
  target_link_libraries(minihost PRIVATE ${CMAKE_DL_LIBS})
  enable_testing()
  set(GF $<TARGET_FILE:GateFlare>)
  set(CLIP ${CMAKE_BINARY_DIR}/bar.rgb)
  add_test(NAME make_clip COMMAND make_test_clip 192 108 ${CLIP})
  set_tests_properties(make_clip PROPERTIES FIXTURES_SETUP clip)
  # Nothing to do (no flare, no crop): the picture comes back untouched.
  add_test(NAME off_is_identity
           COMMAND minihost ${GF} ${CLIP} 192 108 ${CMAKE_BINARY_DIR}/off.rgb --context filter
                   --set intensity=0 --set edgeGlow=0 --set cropToGate=0
                   --expect-identity-at 0:Source --expect-identity-at 30:Source)
  # Flare only (no crop): frames with the bar far from the edges are untouched
  # away from the bar... and the frame where it sits just beyond the top of the
  # gate is brighter than the plain frame. Display encoding: never past white.
  add_test(NAME flare_from_margin
           COMMAND minihost ${GF} ${CLIP} 192 108 ${CMAKE_BINARY_DIR}/flare.rgb --context filter
                   --set cropToGate=0 --set gateCrop=10 --set isolateLights=0
                   --expect-mean-above 28:0.175 --max-below 1.0000001)
  # Isolate Lights with a small Light Size: the bar is bigger than a "light",
  # so it doesn't flare (like a lit wall).
  add_test(NAME isolate_ignores_big_areas
           COMMAND minihost ${GF} ${CLIP} 192 108 ${CMAKE_BINARY_DIR}/iso.rgb --context filter
                   --set cropToGate=0 --set gateCrop=10 --set isolateLights=1 --set lightSize=1
                   --expect-mean-below 28:0.135)
  # Same frame with the flare off, for comparison (the check above must fail here).
  add_test(NAME no_flare_baseline
           COMMAND minihost ${GF} ${CLIP} 192 108 ${CMAKE_BINARY_DIR}/base.rgb --context filter
                   --set cropToGate=0 --set gateCrop=10 --set intensity=0 --set edgeGlow=0
                   --expect-mean-below 28:0.175)
  # Crop to gate, scene-referred encoding, Show Gate: all render and stay deterministic.
  add_test(NAME crop_and_options
           COMMAND minihost ${GF} ${CLIP} 192 108 ${CMAKE_BINARY_DIR}/crop.rgb --context filter
                   --set encoding=7 --set gateCrop=8 --set fan=2 --set sourceColour=0)
  add_test(NAME show_gate
           COMMAND minihost ${GF} ${CLIP} 192 108 ${CMAKE_BINARY_DIR}/show.rgb --context filter --set showGate=1)
  # Each shape (Beam, Blade) flares from the margin too, with a colour preset.
  foreach(SHAPE 1 2)
    add_test(NAME shape_${SHAPE}
             COMMAND minihost ${GF} ${CLIP} 192 108 ${CMAKE_BINARY_DIR}/shape${SHAPE}.rgb --context filter
                     --set cropToGate=0 --set gateCrop=10 --set isolateLights=0 --set shape=${SHAPE}
                     --set flareColour=${SHAPE} --expect-mean-above 28:0.128 --max-below 1.0000001)
    set_tests_properties(shape_${SHAPE} PROPERTIES FIXTURES_REQUIRED clip)
  endforeach()
  # Character off (smooth, no shimmer): still flares; with heavy streaks,
  # shimmer and fringe: still flares, deterministic, never past white.
  add_test(NAME character_off
           COMMAND minihost ${GF} ${CLIP} 192 108 ${CMAKE_BINARY_DIR}/charoff.rgb --context filter
                   --set cropToGate=0 --set gateCrop=10 --set isolateLights=0 --set streaks=0 --set shimmer=0
                   --set core=0 --set fringe=0 --expect-mean-above 28:0.14)
  add_test(NAME character_heavy
           COMMAND minihost ${GF} ${CLIP} 192 108 ${CMAKE_BINARY_DIR}/charheavy.rgb --context filter
                   --set cropToGate=0 --set gateCrop=10 --set isolateLights=0 --set streaks=2 --set shimmer=1
                   --set shimmerSpeed=5 --set fringe=2 --set core=1 --set seed=7 --set shape=2
                   --expect-mean-above 28:0.14 --max-below 1.0000001)
  # Film composite (default): even a very strong flare stays strictly below
  # white. Screen and Add composites render too.
  add_test(NAME film_composite_never_clips
           COMMAND minihost ${GF} ${CLIP} 192 108 ${CMAKE_BINARY_DIR}/film.rgb --context filter
                   --set cropToGate=0 --set gateCrop=10 --set isolateLights=0 --set intensity=8 --set core=1
                   --expect-mean-above 28:0.15 --max-below 1.0)
  add_test(NAME other_composites
           COMMAND minihost ${GF} ${CLIP} 192 108 ${CMAKE_BINARY_DIR}/scr.rgb --context filter
                   --set cropToGate=0 --set gateCrop=10 --set isolateLights=0 --set composite=1)
  add_test(NAME add_composite_log
           COMMAND minihost ${GF} ${CLIP} 192 108 ${CMAKE_BINARY_DIR}/add.rgb --context filter
                   --set cropToGate=0 --set gateCrop=10 --set isolateLights=0 --set composite=2 --set encoding=7)
  # Flare Start: at frame 28 the bar is just past the top of the gate, so a
  # Flare Start of -8% (only flare well past the edge) gives no flare yet.
  add_test(NAME flare_start_later
           COMMAND minihost ${GF} ${CLIP} 192 108 ${CMAKE_BINARY_DIR}/late.rgb --context filter
                   --set cropToGate=0 --set gateCrop=10 --set isolateLights=0 --set flareStart=-8 --set edgeGlow=0
                   --expect-mean-below 28:0.13)
  add_test(NAME flare_start_earlier
           COMMAND minihost ${GF} ${CLIP} 192 108 ${CMAKE_BINARY_DIR}/early.rgb --context filter
                   --set cropToGate=0 --set gateCrop=10 --set isolateLights=0 --set flareStart=5
                   --expect-mean-above 20:0.13)
  # Looks: a preset on the top & bottom pair changes the flare; Blend mixes
  # two looks; the left & right pair leaves a top-edge flare alone; both pairs
  # at zero intensity (and no crop) is a pass-through.
  add_test(NAME top_look_preset
           COMMAND minihost ${GF} ${CLIP} 192 108 ${CMAKE_BINARY_DIR}/lookp.rgb --context filter
                   --set cropToGate=0 --set gateCrop=10 --set isolateLights=0 --set topLookA=5
                   --expect-mean-above 28:0.15 --max-below 1.0)
  add_test(NAME look_blend
           COMMAND minihost ${GF} ${CLIP} 192 108 ${CMAKE_BINARY_DIR}/blend.rgb --context filter
                   --set cropToGate=0 --set gateCrop=10 --set isolateLights=0 --set topLookB=5 --set topBlend=0.5
                   --expect-mean-above 28:0.17 --expect-mean-below 28:0.22)
  add_test(NAME pairs_independent
           COMMAND minihost ${GF} ${CLIP} 192 108 ${CMAKE_BINARY_DIR}/pairs.rgb --context filter
                   --set cropToGate=0 --set gateCrop=10 --set isolateLights=0 --set topIntensity=0 --set sideLookA=5
                   --expect-mean-below 28:0.135)
  add_test(NAME pairs_off_identity
           COMMAND minihost ${GF} ${CLIP} 192 108 ${CMAKE_BINARY_DIR}/pairsoff.rgb --context filter
                   --set cropToGate=0 --set topIntensity=0 --set sideIntensity=0 --expect-identity-at 28:Source)
  # Pinch shape (Custom) and the Side Blast preset (a pinch) both render a
  # flare that never clips.
  add_test(NAME pinch_shape
           COMMAND minihost ${GF} ${CLIP} 192 108 ${CMAKE_BINARY_DIR}/pinch.rgb --context filter
                   --set cropToGate=0 --set gateCrop=10 --set isolateLights=0 --set shape=3 --set lean=10
                   --expect-mean-above 28:0.14 --max-below 1.0)
  add_test(NAME side_blast_top
           COMMAND minihost ${GF} ${CLIP} 192 108 ${CMAKE_BINARY_DIR}/blastt.rgb --context filter
                   --set cropToGate=0 --set gateCrop=10 --set isolateLights=0 --set topLookA=5 --set topStartOffset=0.8
                   --set topFocusOffset=5 --expect-mean-above 28:0.14 --max-below 1.0)
  set_tests_properties(pinch_shape side_blast_top PROPERTIES FIXTURES_REQUIRED clip)
  # Veil, iridescence and curve render a flare that never clips.
  add_test(NAME organic_controls
           COMMAND minihost ${GF} ${CLIP} 192 108 ${CMAKE_BINARY_DIR}/orgc.rgb --context filter
                   --set cropToGate=0 --set gateCrop=10 --set isolateLights=0 --set veil=1 --set iridescence=1
                   --set rainbowAlong=0.5 --set prismEdges=0.5 --set curve=1 --set falloff=1
                   --expect-mean-above 28:0.14 --max-below 1.0)
  set_tests_properties(organic_controls PROPERTIES FIXTURES_REQUIRED clip)
  set_tests_properties(top_look_preset look_blend pairs_independent pairs_off_identity
                       PROPERTIES FIXTURES_REQUIRED clip)
  set_tests_properties(flare_start_later flare_start_earlier PROPERTIES FIXTURES_REQUIRED clip)
  set_tests_properties(character_off character_heavy film_composite_never_clips other_composites add_composite_log
                       PROPERTIES FIXTURES_REQUIRED clip)
  set_tests_properties(off_is_identity flare_from_margin no_flare_baseline crop_and_options show_gate
                       isolate_ignores_big_areas
                       PROPERTIES FIXTURES_REQUIRED clip)
endif()
