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
As with the project source developer, all my development is with various Raspberry Pi boards with Infinitive running on a Pi 4. A Pi Zero would probably do fine, except it has no USB ports (continue reading). The Pi 4 also runs PiHole and HomeBridge with CPU loads of just a few percent. All early work was done under Linux 10 (Buster). No issues found moving to Bullseye. Moving to Bookworm on a Pi5 resulted in a failed build, suspect the change to GCC version. The build failed with:

* gcc: error: unrecognized command-line option '-marm'

Wiring to the Carrier HVAC employs solid core multi-conductor wire as intended for the purpose, per the referenced GitHub project. In my case, the wire is run adjacent to network and alarm system wires in the basement for a distance of perhaps 20-25 feet up to the ceiling, across, and down to my network equipment.

Found two alternatives while shopping for the RS-485 interface, TTL and USB. The original Infinitive developer used USB and reported on a driver lockup issue and suggested using USB 1 mode. The HASS version author provided instructions for running Infinitive under systemd. systemd is highly recommended, it solves any restart issue with the app or the driver and cleanly manages startup under any condition. Two alterative interfaces identified and purchased were:
* (eBay) TTL RS-485 interface:	dlymore TTL Serial Port to RS485 Converter Module, was less than $7.45 with shipping.
* (eBay) USB interface:	U-485 USB RS485 Serial Port Converter, was $8.91 with shipping.

Initially used the RS-485 to TTL adapter wired directly to the serial pins on the GPIO bus which worked as soon as the wires to the HVAC were connected in January 2023. Immediately found the restart issue in the serial driver. The restarts are frequent and found to occur between minutes to after many hours of operation. The code was adapted to these frequent restarts. Much later switched to the USB interface and found it to be stable over days and have observed no decribed driver lockup incidents. While it would have permitted a simpler code design, the path taken was educational.

Below is the RS-485 to TTL installation while running Buster:
![SK_RPi4_InfinitivePiholeHomeBridge](https://github.com/skutoroff/Infinitive-Carrier-HVAC-Enhanced/assets/7796742/19ddfaa0-1728-4202-bb1f-d3513628fa46)

Here is the currently employed and trouble fee RS-485 to USB installation (used with both Buster and Bullseye):
![RPi4 - Pihole, Infinitive, HomeBridge](https://github.com/skutoroff/Infinitive-Carrier-HVAC-Enhanced/assets/7796742/815b2c45-3293-4887-b96b-e94e5250f19e)

There is one use-case for the TTL interface. Infinitive should run quite well on a Pi Zero W. Adding a header to the GPIO bus and using the TTL interface avoids the need for a micro-USB to USB-A adapter. The work around for the restarts in collecting data and systemd supports the TTL interface well. Recently (2023-07-08), investigated a Pi Zero WH with TTL adapter under Bullseye to see if the OS update fixed the serial port issue. The serial ttyS0 interface caused a restart within the first hour. So it goes. Below is a photo of the test setup:
![66EED6B9-CF5F-4934-AA55-1D588219E0A5_1_105_c](https://github.com/skutoroff/Infinitive-Carrier-HVAC-Enhanced/assets/7796742/49ce5bc9-0c30-41df-8311-b8b5a3c7527f)

Summary, even if you enjoy wiring stuff up and want to use the GPIO pins, don't bother with the TTL option unless you want to use a Pi Zero W (with header).

#### Software

Started with go version 1.20.2 linux/arm, now using 1.21.5. The planned enhancements required periodic time based execution. Found [cron v3](https://github.com/robfig/cron) which provides a cron-like time specification. It is used to collect temperature and fan readings at 4 minute intervals. Another cron timer saves daily data to files and then prepares a basic daily chart using [Go E-charts](https://github.com/go-echarts/go-echarts). First pass at charting was pretty simple. Lots is left to learn about the e-chart project. Another timer clears the log files 2x per month.
The error log file grew fast with the TTL interface; with the USB interface it contains only added error and progress messages. Still, log file purges are deemed useful. Lowered the log level anyway.

As for the time axis in the charts, have not yet figured out how to set up text format time in the scale. So, the daily data include a date.dayFraction representation of time for the time scale. Just wanted to see a chart more than I wanted to learn e-charts at the time. May later adapt to a more Julian date style. Below is an early version of the chart. The one restart was a software update.
![2023-06-07_Chart](https://github.com/skutoroff/Infinitive-Carrier-HVAC-Enhanced/assets/7796742/988c611f-15f8-4f63-83ff-301a5c5c855a)

Code adding the axis names works, but it places the Y-axis name above the axis line which puts it under/over the chart subtitle. The X-axis name is just to the right of the axis line. Have not found suitable example code to mimic that both builds and places the axis names in middle of the X-axis and vertical for the Y-axis, etc. Examples found to date are very basic, more educational rather than complete (IMHO). Originally, the daily chart was only produced just before midnight. Current version produces partial charts during the day at 06:00, 08:00, 10:00, 12:00 14:00, 16:00 18:00, 20:00, and lastly at midnight.

Minor changes 2023-09-14. Changed chart file name from yyyy-mm-dd_Chart.html to yyyy-mm-dd_Infinitive.html as the filename extension is all that is needed to distinguish them. Addtional minor code change to charting, but it has impact on appearance. Change made for completeness as investigation in axis label placement is paused.

Below is one from the 16:00 run (updated 2023-07-22 added unit duty cyce % to subtitle).
* My AC is amazingly powerful. When running, it actually changes the outdoor temperature! I ought to shade the condenser from the sun.
![Screenshot 2023-07-22 at 16 34 21](https://github.com/skutoroff/Infinitive-Carrier-HVAC-Enhanced/assets/7796742/66250395-feb6-4bdc-abef-b003b03f1bfa)

As noted, Infinitive runs under systemd. Added redirection of output and error logs to /var/log/infinitive/. Infinitive is run from /var/lib/infinitive/ with data and chart files also saved there. Data files are in CSV form allowing import into Excel.
The blower RPM scale is the reported fan speed converted to off-low-med-high scale as 0, 34, 66, 100 to use the same y scale as temperature. Temperature readings and blower RPM readings are sometimes corrupted in the RS-485 transmission, the code cleans up the obvious exteme errors. One day blower RPM will be shown with a right side scale. Changing the time scale to be text date/time is also intended. Axis name placement needs to be improved. So it goes...

The problem building the web user interface was resolved with recent (Deceber 2023) updates to the orignal Infinitive project. This work is ongoing.
![Saved Measurements Table](https://github.com/skutoroff/Infinitive-Carrier-HVAC-Enhanced/assets/7796742/4ce59f0f-33d4-426d-be79-b8e8b0e4ddcb)

To use the executable Pi file, install it in /var/lib/infinitive/ and set it up in systemd, see the second link at top. If you configure systemd to save output and error log files, save them in /var/log/infinitive/ as in the sample infinitive.service file and they will be deleted 2x per month to manage their size.

#### Minor Nonsense
With no formal Go experience, I like Go better than many other programming languages I’ve used. I like that Go programs are a single complete executable with no additional support files. I like the sort of C like resemblance and the way objects are referenced. The object-method chaining is kind of neat, but hindered readability at first. Wonder about using the USB RS-485 interface on a Mac and building for macos. Just a thought, no serious interest in doing it.

#### Updates In Progress
1. Revising code to handle multiple year data collection and charting.
2. Revisng the UI is just now underway as the source project changed. In progress.

![2023-12-24 Year_2023-121](https://github.com/skutoroff/Infinitive-Carrier-HVAC-Enhanced/assets/7796742/bb037429-c708-49dc-a50f-cd958e10c501)
