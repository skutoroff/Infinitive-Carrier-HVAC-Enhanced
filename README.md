# Infinitive Carrier HVAC - Enhanced
Based on the project acd/infinitive:

* Reference:	https://github.com/acd/infinitive
* See also:	https://github.com/mww012/hass-infinitive

Start with the infinitive project before further reading here.

The original disclaimer still applies. Maybe more so. (copied from the Infinitive README.md)

## **DISCLAIMER**
**The software and hardware described here interacts with a proprietary system using information gleaned by reverse engineering.  Although it works fine for some, no guarantee or warranty is provided.  Use at your own risk to your HVAC system and yourself.**

## My Infinitive Goals
1. Learn Go.
2. Collect HVAC temperature and blower rpm data to chart to visualize HVAC cycle times for no reason other than something to do.
3. Have some geeky fun.

## Getting started

#### Hardware setup
As with the project source developer, all my development is with various Raspberry Pi boards with Infinitive running on a Pi 4. A Pi Zero would probably do fine, except it has no USB ports (continue reading). The Pi 4 also runs PiHole and HomeBridge with CPU loads of just a few percent. All early work was done under Linux 10 (Buster). No issues found moving to Bullseye. Updated this enhancement to source project updates (2023-12). They built easy after updating to Go 1.21.5. Building under Bookworm on a Pi5 requires the ARM 64bit version `go1.21.5.linux-arm64.tar.gz`, not the 6l version used on the Pi 4.

Wiring to the Carrier HVAC employs solid core multi-conductor wire as intended for the purpose, per the referenced GitHub project. In my case, the wire is run adjacent to network and alarm system wires in the basement for a distance of perhaps 20-25 feet up to the ceiling, across, and down to my network equipment.

Found two alternatives while shopping for the RS-485 interface, TTL and USB. The original Infinitive developer used USB and reported on a driver lockup issue and suggested using USB 1 mode. The HASS version author provided instructions for running Infinitive under systemd. systemd is highly recommended, it solves any restart issue with the app or the driver and cleanly manages startup under any condition. Two alterative interfaces identified and purchased were:
* (eBay) TTL RS-485 interface:	dlymore TTL Serial Port to RS485 Converter Module, was less than $7.45 with shipping.
* (eBay) USB interface:	U-485 USB RS485 Serial Port Converter, was $8.91 with shipping.

Initially used the RS-485 to TTL adapter wired directly to the serial pins on the GPIO bus which worked as soon as the wires to the HVAC were connected in January 2023. Immediately found the restart issue in the serial driver. The restarts are frequent and found to occur between minutes to after many hours of operation. The code was adapted to these frequent restarts. Much later switched to the USB interface and found it to be stable over days and have observed no decribed driver lockup incidents. While it would have permitted a simpler code design, the path taken was educational.

Below is the RS-485 to TTL installation while running Buster:
![SK_RPi4_InfinitivePiholeHomeBridge](https://github.com/skutoroff/Infinitive-Carrier-HVAC-Enhanced/assets/7796742/19ddfaa0-1728-4202-bb1f-d3513628fa46)

Below is the currently employed and trouble fee RS-485 to USB installation (used with both Buster and Bullseye):
![RPi4 - Pihole, Infinitive, HomeBridge](https://github.com/skutoroff/Infinitive-Carrier-HVAC-Enhanced/assets/7796742/815b2c45-3293-4887-b96b-e94e5250f19e)

There is one use-case for the TTL interface. Infinitive should run quite well on a Pi Zero W. Adding a header to the GPIO bus and using the TTL interface avoids the need for a micro-USB to USB-A adapter. The work around for the restarts in collecting data and systemd supports the TTL interface well. Recently (2023-07-08), investigated a Pi Zero WH with TTL adapter under Bullseye to see if the OS update fixed the serial port issue. The serial ttyS0 interface caused a restart within the first hour. So it goes. Below is a photo of the test setup:
![66EED6B9-CF5F-4934-AA55-1D588219E0A5_1_105_c](https://github.com/skutoroff/Infinitive-Carrier-HVAC-Enhanced/assets/7796742/49ce5bc9-0c30-41df-8311-b8b5a3c7527f)

Summary, even if you enjoy wiring stuff up and want to use the GPIO pins, don't bother with the TTL option unless you want to use a Pi Zero W (with header).

### Software

Started with go version 1.20.2 linux/arm, now using 1.21.5.
Doing builds on both a Pi4 Bullseye and a Pi5 Bookworm.
Besides the installing the correct go toolset (above), a build on Bookworm does not run on Bullseye, get a file not found error, cause unknown.
This can be fixed by cross-compiling as (there may be better ways to do this):

`env GOOS=linux GOARCH=arm GOARM=6 go build -ldflags "-X main.Version=0.4.1.06"`

The planned enhancements required periodic time based execution. Found [cron v3](https://github.com/robfig/cron) which provides a cron-like time specification. It is used to collect temperature and fan readings at 4 minute intervals.
Another cron timer saves daily data to files and then prepares a basic daily chart using [Go E-charts](https://github.com/go-echarts/go-echarts). First pass at charting was pretty simple. Lots is left to learn about the e-chart project.
Another timer clears the log files 2x per month.
The error log file grew fast with the TTL interface; with the USB interface it contains only added error and progress messages. Still, log file purges are deemed useful. Lowered the log level anyway.

As noted, Infinitive runs under systemd.
Copy the `.service` file to `/etc/systemd/system/`. Then run `sudo systemctl enable infinitive.service` followed by `sudo systemctl start infinitive.service`.
Redirection of output and error logs to `/var/log/infinitive/` is included in the `.service` file.
Be sure to create the folders.
Infinitive is run from `/var/lib/infinitive/` with data and chart files also saved there with sub-folders for each month of collection.
Data files are in CSV form allowing import into Excel.

As for the time axis in the charts, have not yet figured out how to set up text format time in the scale.
So, the daily data include a date.dayFraction representation of time for the time scale.
Just wanted to see a chart more than I wanted to learn e-charts at the time.
May later adapt to a more Julian date style. Below is an early version of the chart.
The blower RPM scale is the reported fan speed converted to off-low-med-high scale as 0, 34, 66, 100 to use the same y scale as temperature.
Temperature readings and blower RPM readings are sometimes corrupted in the RS-485 transmission, the code cleans up the obvious exteme errors.
In the future, blower RPM will be shown with a right side scale.
Changing the time scale to be text date/time is also intended.
Axis name placement needs to be improved.
![2023-06-07_Chart](https://github.com/skutoroff/Infinitive-Carrier-HVAC-Enhanced/assets/7796742/988c611f-15f8-4f63-83ff-301a5c5c855a)

Code adding the axis names works, but it places the Y-axis name above the axis line which puts it under/over the chart subtitle.
The X-axis name is just to the right of the axis line.
Have not found suitable example code to mimic that both builds and places the axis names in middle of the X-axis and vertical for the Y-axis, etc.
Examples found to date are very basic, more educational rather than complete (IMHO).
Originally, the daily chart was only produced just before midnight.
Current version produces partial charts during the day at 06:00, 08:00, 10:00, 12:00 14:00, 16:00 18:00, 20:00, and lastly at midnight.

Minor changes 2023-09-14. Changed chart file name from `yyyy-mm-dd_Chart.html` to `yyyy-mm-dd_Infinitive.html` as the filename extension is all that is needed to distinguish the files.
Added the blower percent on time to the subheading and later used this data for additonal historical charting over the year.
![2024-01-02 One Day](https://github.com/skutoroff/Infinitive-Carrier-HVAC-Enhanced/assets/7796742/3af765ac-c6ca-45ab-aa58-3e29ebb5889c)

The blower % on time data is extracted to show change in HVAC operation over the year.
I would like to distinguish heat from cold by changing the line color, maybe in the future.

To use the executable Pi file, install it in  `/var/lib/infinitive/` and set it up in systemd.
If you configure systemd to save output and error log files, save them in `/var/log/infinitive/` as in the sample infinitive.service file and they will be deleted 2x per month to manage their size.

### Updates January-February 2024

Infinitive modifications now handle multiple year data collection and charting.
There may still be bugs to be found as current year data progresses into the prior year on the chart.
The code should enforce a 15 day separation berween new data from the left and existing prior year to the right.
The gap and missing data show as missing data (lost files or files before the %on was calculated).
Below is a chart showing the current year 2024 on the left and the prior year to the right.
I'd like to show the heating and cooling line segments in different colors and also annotate the current and prior year areas, e-charts remains a mystery.

![Screenshot 2024-04-12 at 11 13 23](https://github.com/skutoroff/Infinitive-Carrier-HVAC-Enhanced/assets/7796742/8a8e3c76-c5cb-48fc-b4fb-31cbd9a4a3c1)

It was always planned to make the charts available through the UI.
January update added a static file HTML server application based on generally available education code.
The HTML file with the links has been renamed to index.html.
The links are file name matches to work with the static server code.
This table and the charts can now be viewed from any computer on the local network using the IP address and port 8081, as http://yo.ur.i.p:8081/infinitive/index.html
The new release of original project source permits UI modifications as the dependence on `bindata_assets` is gone. Neat. I'll have to mess with that sometime.

![Screenshot 2024-02-15 at 08 54 59](https://github.com/skutoroff/Infinitive-Carrier-HVAC-Enhanced/assets/7796742/0f86fc9d-f7bb-41b0-a0d4-6edf000ea387)

As time permits, may improve the chart apearance. Also, for the exceptional cases when data collection stops or pauses, may change the daily charts to show missing data gaps.
This han't been an issue since getting things working, but still...

### 2024-09-20 Update

The Feb 14 version has been running trouble free for months.

I've moved the project to a Pi 400 and the new "USB to Multi-Protocol Serial Adapter: RS-232/TTL UART/RS-485" from Adafruit.com.
Plan to use the Pi4 for a camera project.
The new USB-serial interface is 3x the price of the one from eBay and it is unlikely I will ever use the other serial port options.
One negative for this device, the screws are really tiny.
On the plus side, the data LED is green.
This is a plus because the blue LED on the eBay one really lit up the room and was annoying at night.
Not a problem until we finsihed the basement this year.

![2F8EACE8-5F7E-4A31-89AB-3E8DD2E7C89C_1_105_c](https://github.com/user-attachments/assets/6ea62864-9285-4b70-8765-f38c898c6ef0)

I'm also considering opening up the interface port for limited external (non-loacl) access.
This is risky so I looked into adding guardrail code to limit changes from non-local IP addresses.
First step was to add guardrail code to the file: internal/assets/app/app.js.
It works, but he source can easily be viewed and changed in a browser debug window.
I may upload the updated app and file shortly.

#### Minor Nonsense

With no formal Go experience, I like Go better than many other programming languages I’ve used.
I like that Go programs are a single complete executable with no additional support files.
Creating concurrent execution units was easy, as done for the static file server on port 8081 while the UI runs on 8080.
I like the sort of C like resemblance and the way objects are referenced.
The object-method chaining is kind of neat, but hindered readability at first.
Go almost makes me wish I was back to work as a code monkey.

Wonder about using the USB RS-485 interface on a Mac and building for macos. Just a thought, no serious interest in doing it.

