cmake_minimum_required(VERSION 3.16)

# Universal (Apple silicon + Intel) by default on macOS. Must be set before project().
if(APPLE AND NOT CMAKE_OSX_ARCHITECTURES)
  set(CMAKE_OSX_ARCHITECTURES "arm64;x86_64" CACHE STRING "" FORCE)
endif()
set(CMAKE_OSX_DEPLOYMENT_TARGET "11.0" CACHE STRING "Minimum macOS")

project(FilmFlash VERSION 1.5.0 LANGUAGES CXX)

set(CMAKE_CXX_STANDARD 17)
set(CMAKE_CXX_STANDARD_REQUIRED ON)
if(NOT CMAKE_BUILD_TYPE)
  set(CMAKE_BUILD_TYPE Release)
endif()

find_package(Threads REQUIRED)
set(OFX_INCLUDE ${CMAKE_CURRENT_SOURCE_DIR}/../third_party/openfx/include)

add_library(flash_engine STATIC src/FlashEngine.cpp)
target_include_directories(flash_engine PUBLIC src)
set_target_properties(flash_engine PROPERTIES POSITION_INDEPENDENT_CODE ON
                                              CXX_VISIBILITY_PRESET hidden)
target_link_libraries(flash_engine PUBLIC Threads::Threads)

if(APPLE)
  set(OFX_ARCH_DIR MacOS)
elseif(WIN32)
  set(OFX_ARCH_DIR Win64)
else()
  set(OFX_ARCH_DIR Linux-x86-64)
endif()
set(BUNDLE_DIR ${CMAKE_BINARY_DIR}/FilmFlash.ofx.bundle)

add_library(FilmFlash MODULE src/FilmFlashPlugin.cpp)
target_include_directories(FilmFlash PRIVATE ${OFX_INCLUDE})
target_link_libraries(FilmFlash PRIVATE flash_engine)
set_target_properties(FilmFlash PROPERTIES
  PREFIX "" SUFFIX ".ofx"
  CXX_VISIBILITY_PRESET hidden
  VISIBILITY_INLINES_HIDDEN ON
  LIBRARY_OUTPUT_DIRECTORY ${BUNDLE_DIR}/Contents/${OFX_ARCH_DIR})
if(NOT MSVC)
  target_compile_options(FilmFlash PRIVATE -Wall -Wextra)
  target_compile_options(flash_engine PRIVATE -Wall -Wextra)
endif()
if(APPLE)
  configure_file(resources/Info.plist.in ${BUNDLE_DIR}/Contents/Info.plist @ONLY)
endif()

add_executable(flash_cli tools/flash_cli.cpp)
target_link_libraries(flash_cli PRIVATE flash_engine)

if(NOT WIN32)
  add_executable(minihost ${CMAKE_CURRENT_SOURCE_DIR}/../ofx-testhost/minihost.cpp)
  target_include_directories(minihost PRIVATE ${OFX_INCLUDE})
  target_link_libraries(minihost PRIVATE ${CMAKE_DL_LIBS})
  enable_testing()
  set(FF $<TARGET_FILE:FilmFlash>)
  # Transition, defaults: untouched at both ends, near white at the cut, never clipped.
  add_test(NAME transition_defaults
           COMMAND minihost ${FF} synthetic 192 108 ${CMAKE_BINARY_DIR}/t_default.rgb --context transition
                   --expect-identity-at 0:SourceFrom --expect-identity-at 47:SourceTo
                   --expect-mean-above 23:0.85 --expect-mean-below 5:0.75 --max-below 1.0)
  # A brutal 14-stop peak (18 with uneven exposure) with everything on: it gets
  # as close to white as floats can tell apart, but never past it.
  add_test(NAME transition_extreme
           COMMAND minihost ${FF} synthetic 192 108 ${CMAKE_BINARY_DIR}/t_extreme.rgb --context transition
                   --set peak=14 --set hold=0.3 --set fog=2 --set halation=3 --set leak=3
                   --set unevenness=4 --set flicker=2 --set cutSoftness=0.2 --set seed=9
                   --expect-identity-at 0:SourceFrom --expect-identity-at 47:SourceTo --max-below 1.0000001)
  # Scene-referred input (DaVinci Intermediate).
  add_test(NAME transition_davinci_intermediate
           COMMAND minihost ${FF} synthetic 192 108 ${CMAKE_BINARY_DIR}/t_di.rgb --context transition
                   --set encoding=5 --expect-identity-at 0:SourceFrom --expect-identity-at 47:SourceTo
                   --expect-mean-above 23:0.6)
  # The transition is flash-only (Resolve gives transitions only the current
  # frame): it never reads other times, even with clips that have handles.
  add_test(NAME transition_reads_current_frame_only
           COMMAND minihost ${FF} synthetic 192 108 ${CMAKE_BINARY_DIR}/t_cur.rgb --context transition
                   --offset 12 --frames 24
                   --expect-identity-at 0:SourceFrom --expect-identity-at 23:SourceTo --max-below 1.0)
  add_test(NAME effect_motion_blur
           COMMAND minihost ${FF} synthetic 192 108 ${CMAKE_BINARY_DIR}/e_blur.rgb --context filter
                   --set timing=0 --set flash=3 --set motionBlur=1 --expect-fetches-above 2 --max-below 1.0)
  # As an effect with keyframed Flash: 0 leaves the clip alone; a big flash brightens without clipping.
  add_test(NAME effect_zero_is_identity
           COMMAND minihost ${FF} synthetic 192 108 ${CMAKE_BINARY_DIR}/e_zero.rgb --context filter
                   --set timing=0 --expect-identity-at 0:Source --expect-identity-at 30:Source)
  add_test(NAME effect_flash
           COMMAND minihost ${FF} synthetic 192 108 ${CMAKE_BINARY_DIR}/e_flash.rgb --context filter
                   --set timing=0 --set flash=9 --expect-mean-above 10:0.85 --max-below 1.0)
  # Run-out on the outgoing clip: clean until the last 16 frames, peak on the
  # last frame, speed-up reading at most 8 frames past the clip's end.
  add_test(NAME effect_run_out
           COMMAND minihost ${FF} synthetic 192 108 ${CMAKE_BINARY_DIR}/e_out.rgb --context filter
                   --frames 40 --set timing=1 --set flashLength=16 --set speedRamp=16 --set spareHandles=8
                   --expect-identity-at 0:Source --expect-identity-at 23:Source --expect-identity-at 24:Source
                   --expect-mean-above 39:0.8 --expect-fetches-above 1.3 --expect-no-failed-fetches --max-below 1.0)
  # Speed Ramp longer than the flash: the action speeds up before the flash
  # starts (frames 8-23 differ from the source), and stays inside the handles.
  add_test(NAME effect_run_out_long_ramp
           COMMAND minihost ${FF} synthetic 192 108 ${CMAKE_BINARY_DIR}/e_ramp.rgb --context filter
                   --frames 40 --set timing=1 --set flashLength=16 --set speedRamp=32 --set spareHandles=8
                   --expect-identity-at 0:Source --expect-identity-at 7:Source
                   --expect-fetches-above 1.3 --expect-no-failed-fetches)
  # Run-up on the incoming clip: peak on the first frame, clean from frame 16,
  # reading at most 8 frames before the clip's start.
  add_test(NAME effect_run_up
           COMMAND minihost ${FF} synthetic 192 108 ${CMAKE_BINARY_DIR}/e_up.rgb --context filter
                   --offset 8 --frames 40 --set timing=2 --set flashLength=16 --set speedRamp=16 --set spareHandles=8
                   --expect-identity-at 15:Source --expect-identity-at 39:Source --expect-mean-above 0:0.8
                   --expect-fetches-above 1.3 --expect-no-failed-fetches --max-below 1.0)
endif()
