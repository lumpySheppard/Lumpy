cmake_minimum_required(VERSION 3.16)

# Universal (Apple silicon + Intel) by default on macOS. Must be set before project().
if(APPLE AND NOT CMAKE_OSX_ARCHITECTURES)
  set(CMAKE_OSX_ARCHITECTURES "arm64;x86_64" CACHE STRING "" FORCE)
endif()
set(CMAKE_OSX_DEPLOYMENT_TARGET "11.0" CACHE STRING "Minimum macOS")

project(GateFlare VERSION 1.0.0 LANGUAGES CXX)

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
                   --set cropToGate=0 --set gateCrop=10 --set intensity=3
                   --expect-mean-above 28:0.175 --max-below 1.0000001)
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
  set_tests_properties(off_is_identity flare_from_margin no_flare_baseline crop_and_options show_gate
                       PROPERTIES FIXTURES_REQUIRED clip)
endif()
