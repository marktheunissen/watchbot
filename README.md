![WatchBot Logo](https://raw.githubusercontent.com/marktheunissen/watchbot/master/images/watchbot-sml.jpg)

## Watchbot: AI on video streams

- Uses [Movidius Neural Compute Stick](https://software.intel.com/content/www/us/en/develop/articles/intel-movidius-neural-compute-stick.html) plugged into a Raspberry Pi
- If matching a label (e.g. person, dog, car), post the alert image to a Telegram channel

Other features:

- Schedule on/off times
- Telegram bot provides a control and configuration interface
- Flexible control using Google PubSub messages to turn on & off
- Video scene region of interest masking and cropping
- Healthchecks, heartbeats, alerts when system is down
- Auto recovery when camera connection is lost
- Send system errors to Telegram from the main systemd journal
- Automated restart if frame rate drops
- Telegram rate limiting to prevent flooding

### Dependencies

- Intel Movidius NCS (original version)
- OpenCV
- GoCV
- gstreamer

### Developing

Watchbot needs the Movidius SDK, so it's necessary to develop on Linux. For this I have an Ubuntu VM, 16.04, with a clone of this repo, and go + deps installed there.

Basic setup steps:

- Install Go
- Install the Movidius SDK. Requires SDK version 1, as the Go bindings are not updated yet in 2.
- `make install` on SSD mobilenet from the appzoo
- `make install` on GoCV
