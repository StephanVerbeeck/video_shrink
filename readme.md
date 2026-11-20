## Video Shrink
WARNING: this will lower the quality of the video files! 
If you "commit" them, then those changes become permanent.

This program is intended as example, not as actual finished product (all what you copy or use from this repository is your responsibility)

- The purpose of this program is to conserve diskspace by converting video files in old video codecs or resolutions higher than 720P to HEVC 720p.
- It reduces both the bitrate of video and audio of the videofiles that match the given wildcard (recursive directory search) such that the resulting bitrate is less than 1500bps.
- The conversion is done with FFMPEG which this program will install automatically if it was not yet on your computer.
- during conversion the new file has "_shrink" added to its name (e.g. "my video_shrink.mp4").
- after conversion the old file has "_unshrink" added to its name (e.g. "my video_unshrink.mp4").
- AVI files (and other container format that do not support HEVC) are converted to MP4.
- The given wildcard must include the filename itself (e.g. "C:\video\*" instead of "C:\video")
- the disk may also be a wildcard (e.g. "*:\video\*").  It will then search for that directory on all harddrives (from "A:\video\*" to "Z:\video\*")
- the program keeps track of past progress (so does not start again from the first file when restarted later)
- this program relies on FFPROBE and FFMPEG (installed during runtime when needed)
- The program will stop early when less than 10Gb free space remains.
- Not all video files can be compressed (some have too much noise in them).  When conversion yields larger files than the original than conversion will be aborted after 10 minutes of the file has been converted.
- This program uses hardware acceleration (NVIDIA GPU, AMD iGPU, INTEL iGPU) to unload the cpu so that the computer remains responsive during the conversion.
- use "commit" to delete all old files (those that end with _unshrink) e.g. "video_shrink commit *:\video\*"
- use "check" to detect damaged video files e.g. "video_shrink check *:\video\*" (those are renamed to *_invalid)
- in the directory where the program resides are log files of what happened (these are zipped when older than 1 day and deleted when older than 100 days)
- assume further that the source code is the manual (how things are done in detail)