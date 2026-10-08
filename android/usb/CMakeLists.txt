cmake_minimum_required(VERSION 3.22.1)
project(sdflasher_usb)
set(LIBUSB_PATH "${USB_SOURCE}/vendor/libusb")
add_subdirectory("${USB_SOURCE}/libaums" "${CMAKE_BINARY_DIR}/libaums")
add_subdirectory("${USB_SOURCE}/libusbcommunication" "${CMAKE_BINARY_DIR}/libusbcommunication")

foreach(target errno-lib usb-lib libusb libusbcom)
    target_link_options(${target} PRIVATE "-Wl,-z,common-page-size=16384")
endforeach()
