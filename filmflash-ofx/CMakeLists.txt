cmake_minimum_required(VERSION 3.16)

# Universal (Apple silicon + Intel) by default on macOS. Must be set before project().
if(APPLE AND NOT CMAKE_OSX_ARCHITECTURES)
  set(CMAKE_OSX_ARCHITECTURES "arm64;x86_64" CACHE STRING "" FORCE)
endif()
set(CMAKE_OSX_DEPLOYMENT_TARGET "11.0" CACHE STRING "Minimum macOS")

project(FilmFlash VERSION 1.1.0 LANGUAGES CXX)

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
  # Speed-up and motion blur read other frames (more than one fetch per frame),
  # still leave both ends untouched, and stay deterministic.
  add_test(NAME transition_speed_and_blur
           COMMAND minihost ${FF} synthetic 192 108 ${CMAKE_BINARY_DIR}/t_speed.rgb --context transition
                   --set speedUp=1 --set maxSpeed=8 --set motionBlur=1 --set maxBlur=8
                   --expect-identity-at 0:SourceFrom --expect-identity-at 47:SourceTo
                   --expect-fetches-above 1.5 --max-below 1.0)
  # A 24-frame transition in the middle of 48-frame clips, so speed-up can
  # read into real handles on both sides.
  add_test(NAME transition_with_handles
           COMMAND minihost ${FF} synthetic 192 108 ${CMAKE_BINARY_DIR}/t_handles.rgb --context transition
                   --offset 12 --frames 24 --set maxSpeed=12
                   --expect-identity-at 0:SourceFrom --expect-identity-at 23:SourceTo
                   --expect-fetches-above 1.5 --max-below 1.0)
  # Both off: exactly one fetch per frame and no reads from other times.
  add_test(NAME transition_speed_off
           COMMAND minihost ${FF} synthetic 192 108 ${CMAKE_BINARY_DIR}/t_nospeed.rgb --context transition
                   --set speedUp=0 --set motionBlur=0 --expect-identity-at 0:SourceFrom
                   --expect-identity-at 47:SourceTo)
  add_test(NAME effect_motion_blur
           COMMAND minihost ${FF} synthetic 192 108 ${CMAKE_BINARY_DIR}/e_blur.rgb --context filter
                   --set flash=3 --set motionBlur=1 --expect-fetches-above 2 --max-below 1.0)
  # As an effect: Flash 0 leaves the clip alone; a big flash brightens without clipping.
  add_test(NAME effect_zero_is_identity
           COMMAND minihost ${FF} synthetic 192 108 ${CMAKE_BINARY_DIR}/e_zero.rgb --context filter
                   --expect-identity-at 0:Source --expect-identity-at 30:Source)
  add_test(NAME effect_flash
           COMMAND minihost ${FF} synthetic 192 108 ${CMAKE_BINARY_DIR}/e_flash.rgb --context filter
                   --set flash=9 --expect-mean-above 10:0.85 --max-below 1.0)
endif()
