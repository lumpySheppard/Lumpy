cmake_minimum_required(VERSION 3.16)

# Universal (Apple silicon + Intel) by default on macOS. Must be set before project().
if(APPLE AND NOT CMAKE_OSX_ARCHITECTURES)
  set(CMAKE_OSX_ARCHITECTURES "arm64;x86_64" CACHE STRING "" FORCE)
endif()
set(CMAKE_OSX_DEPLOYMENT_TARGET "11.0" CACHE STRING "Minimum macOS")

project(DataMosh VERSION 1.0.0 LANGUAGES CXX)

set(CMAKE_CXX_STANDARD 17)
set(CMAKE_CXX_STANDARD_REQUIRED ON)
if(NOT CMAKE_BUILD_TYPE)
  set(CMAKE_BUILD_TYPE Release)
endif()

find_package(Threads REQUIRED)
set(OFX_INCLUDE ${CMAKE_CURRENT_SOURCE_DIR}/third_party/openfx/include)

add_library(mosh_engine STATIC src/MoshEngine.cpp)
target_include_directories(mosh_engine PUBLIC src)
set_target_properties(mosh_engine PROPERTIES POSITION_INDEPENDENT_CODE ON
                                             CXX_VISIBILITY_PRESET hidden)
target_link_libraries(mosh_engine PUBLIC Threads::Threads)

# --- The plugin, laid out as an OFX bundle -------------------------------
if(APPLE)
  set(OFX_ARCH_DIR MacOS)
elseif(WIN32)
  set(OFX_ARCH_DIR Win64)
else()
  set(OFX_ARCH_DIR Linux-x86-64)
endif()
set(BUNDLE_DIR ${CMAKE_BINARY_DIR}/DataMosh.ofx.bundle)

add_library(DataMosh MODULE src/DataMoshPlugin.cpp)
target_include_directories(DataMosh PRIVATE ${OFX_INCLUDE})
target_link_libraries(DataMosh PRIVATE mosh_engine)
set_target_properties(DataMosh PROPERTIES
  PREFIX "" SUFFIX ".ofx"
  CXX_VISIBILITY_PRESET hidden
  VISIBILITY_INLINES_HIDDEN ON
  LIBRARY_OUTPUT_DIRECTORY ${BUNDLE_DIR}/Contents/${OFX_ARCH_DIR})
if(NOT MSVC)
  target_compile_options(DataMosh PRIVATE -Wall -Wextra)
  target_compile_options(mosh_engine PRIVATE -Wall -Wextra)
endif()
if(APPLE)
  configure_file(resources/Info.plist.in ${BUNDLE_DIR}/Contents/Info.plist @ONLY)
endif()

# --- Tools and tests ------------------------------------------------------
add_executable(mosh_cli tools/mosh_cli.cpp)
target_link_libraries(mosh_cli PRIVATE mosh_engine)

if(NOT WIN32)
  add_executable(minihost tests/minihost.cpp)
  target_include_directories(minihost PRIVATE ${OFX_INCLUDE})
  target_link_libraries(minihost PRIVATE ${CMAKE_DL_LIBS})
  enable_testing()
  # Synthetic two-shot clip, cut at frame 24, through both modes.
  add_test(NAME minihost_drag
           COMMAND minihost $<TARGET_FILE:DataMosh> synthetic 192 108 ${CMAKE_BINARY_DIR}/drag.rgb
                   --start-here 24 --set residual=0.5)
  add_test(NAME minihost_bloom
           COMMAND minihost $<TARGET_FILE:DataMosh> synthetic 192 108 ${CMAKE_BINARY_DIR}/bloom.rgb
                   --start-here 30 --set mode=1 --set smoothVectors=1 --set heal=0.05 --set mix=0.8)
endif()
