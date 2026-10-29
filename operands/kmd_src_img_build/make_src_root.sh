#!/bin/bash
# -----------------------------------------------------------------------------
#
# Copyright (c) Qualcomm Technologies, Inc. and/or its subsidiaries. All rights reserved.
# Confidential and Proprietary - Qualcomm Technologies, Inc. and/or its subsidiaries.
#
# -----------------------------------------------------------------------------

set -eo pipefail # be more strict about non-zero exits and unset var substitutions

BUILD_TYPE=""
declare -a TOOLS_NEEDED
FW_INPUT=$1
KMD_INPUT=$2


if [ $# != 2 ] ; then
  echo "usage:"
  echo "./make_src_root.sh qaic-fw.rpm qaic-kmd.rpm"
  exit 255
fi

if [ "${FW_INPUT:(-3)}" != "${KMD_INPUT:(-3)}" ] ; then
  echo "$FW_INPUT and $KMD_INPUT do not share the same file extension!"
  exit 255
elif [ "${FW_INPUT:(-3)}" == "rpm" ] ; then
  BUILD_TYPE="rpm"
  TOOLS_NEEDED=( "rpm2cpio" "cpio" )
else
  echo "unknown file extension ${FW_INPUT:(-3)}"
  exit 255
fi
FW_PKG="$(readlink -f "$FW_INPUT")"
KMD_PKG="$(readlink -f "$KMD_INPUT")"

if [[ ! "$FW_PKG" =~ "qaic-fw" ]] ; then
  echo "$FW_INPUT doesn't match 'qaic-fw*'"
  exit 255
fi

if [[ ! "$KMD_PKG" =~ "qaic-kmd" ]] ; then
  echo "$KMD_INPUT doesn't match 'qaic-kmd*'"
  exit 255
fi

if [ ! -f "$FW_PKG" ] || [ ! -f "$KMD_PKG" ] ; then
  echo "no such files: $FW_PKG, $KMD_PKG"
  exit 255
fi


if ! command -v ${TOOLS_NEEDED[0]} &> /dev/null || ! command -v ${TOOLS_NEEDED[1]} &> /dev/null ; then
  echo "cannot support; host does not have necessary tools:"
  echo "${TOOLS_NEEDED[*]}"
  exit 255
fi

SRC_ROOTDIR=$(pwd)/src_rootdir
mkdir -p "$SRC_ROOTDIR"/{firmware,src}
TMP_DIR=$(pwd)/tmp_dir
mkdir -p "$TMP_DIR"
trap 'rm -rf "$TMP_DIR"' EXIT


pushd $TMP_DIR &> /dev/null
case ${BUILD_TYPE} in
"rpm")
  rpm2cpio $FW_PKG | cpio -idmv
  rpm2cpio $KMD_PKG | cpio -idmv
;;
*)
  echo "invalid build-type = ${BUILD_TYPE}"
  rmdir $SRC_ROOTDIR
  popd &> /dev/null
  exit 255
;;
esac
echo "extraction complete to $SRC_ROOTDIR"
cp -r ./lib/firmware/* $SRC_ROOTDIR/firmware
cp -r ./opt/qti-aic/firmware/fw2_swe.json $SRC_ROOTDIR/firmware
cp -r ./usr/src/qaic-*/* $SRC_ROOTDIR/src
popd &> /dev/null