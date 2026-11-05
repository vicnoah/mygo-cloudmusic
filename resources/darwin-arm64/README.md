# Audio Vivid decoder

`molan-av3a` is built from [MOLAN-AV3A](https://github.com/baiji6/MOLAN-AV3A)
0.1.1 (a pure C wrapper around the UWA AVS3-P3 reference decoder). It decodes
NetEase's `av3a` MP4/M4A streams (臻音全景声 / Audio Vivid) to a multichannel
WAV for MPV.

Build (macOS arm64):

```sh
git clone https://github.com/baiji6/MOLAN-AV3A
cd MOLAN-AV3A
# The WAV writer in 0.1.1 writes a low-bit channel mask; for NetEase's
# 7.1.4 (12-channel) output it must be 0x2D63F (FL FR FC LFE BL BR SL SR
# TFL TFR TBL TBR) and for 5.1 (6-channel) 0x3F.
cmake -B build -DCMAKE_BUILD_TYPE=Release
cmake --build build --parallel
cp build/molan-av3a .
```

License: the decoder sources are the AVS3-P3 reference software from
HUAWEI Technologies, governed by the UWA Code Sharing Policy. See
`MOLAN-AV3A.LICENSE` and `MOLAN-AV3A.NOTICE` beside this file. Review those
terms before redistributing the binary.
