# Sample toolchain file for building for Windows from an Ubuntu Linux system.
#
# Typical usage:
#    *) install cross compiler: `sudo apt-get install mingw-w64`
#    *) mkdir buildMingw64 && cd buildMingw64
#    *) cmake -DCMAKE_TOOLCHAIN_FILE=~/Toolchain-Ubuntu-mingw64.cmake ..
#

set(CMAKE_SYSTEM_NAME Windows)
set(GNU_HOST x86_64-w64-mingw32)
#set(GNU_HOST i686-w64-mingw32)
set(TOOLCHAIN_PREFIX x86_64-w64-mingw32)
#set(TOOLCHAIN_PREFIX i686-w64-mingw32)

# cross compilers to use for C and C++
#set(CMAKE_C_COMPILER ${TOOLCHAIN_PREFIX}-gcc)
#set(CMAKE_CXX_COMPILER ${TOOLCHAIN_PREFIX}-g++)
set(CMAKE_C_COMPILER ${TOOLCHAIN_PREFIX}-gcc-posix)
set(CMAKE_CXX_COMPILER ${TOOLCHAIN_PREFIX}-g++-posix)
set(CMAKE_RC_COMPILER ${TOOLCHAIN_PREFIX}-windres)

# host compilers to use for C and C++
set(CMAKE_HOST_C_COMPILER gcc)
set(CMAKE_HOST_CXX_COMPILER g++)

# target environment on the build host system
set(CMAKE_FIND_ROOT_PATH /usr/${TOOLCHAIN_PREFIX} /usr/lib/gcc/${TOOLCHAIN_PREFIX}/7.3-posix)


# modify default behavior of FIND_XXX() commands to
# search for headers/libs in the target environment and
# search for programs in the build host environment
set(CMAKE_FIND_ROOT_PATH_MODE_PROGRAM NEVER)
set(CMAKE_FIND_ROOT_PATH_MODE_LIBRARY ONLY)
set(CMAKE_FIND_ROOT_PATH_MODE_INCLUDE ONLY)
