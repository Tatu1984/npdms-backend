#!/usr/bin/env bash
# Fetch the two OpenCV Zoo model files and verify their SHA-256. Run once on a
# machine with internet access, then copy models/ to an air-gapped edge server.
set -euo pipefail
cd "$(dirname "$0")/models"
ZOO=https://media.githubusercontent.com/media/opencv/opencv_zoo/47534e27c9851bb1128ccc0102f1145e27f23f98/models
fetch() {
  local path=$1 file=$2 sha=$3
  if [[ ! -f $file ]]; then curl -fL --retry 3 -o "$file" "$ZOO/$path/$file"; fi
  echo "$sha  $file" | shasum -a 256 -c -
}
fetch face_detection_yunet    face_detection_yunet_2023mar.onnx   8f2383e4dd3cfbb4553ea8718107fc0423210dc964f9f4280604804ed2552fa4
fetch face_recognition_sface  face_recognition_sface_2021dec.onnx 0ba9fbfa01b5270c96627c4ef784da859931e02f04419c829e83484087c34e79
