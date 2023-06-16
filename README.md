# Infinitive Carrier HVAC - Enhanced
Based on the project acd/infinitive:

* Reference:	https://github.com/acd/infinitive
* See also:	https://github.com/mww012/hass-infinitive

Start with the infinitive project before further reading here.

The original disclaimer still applies. Maybe more so. (copied from the Infinitive README.md)

## **DISCLAIMER**
**The software and hardware described here interacts with a proprietary system using information gleaned by reverse engineering.  Although it works fine for me, no guarantee or warranty is provided.  Use at your own risk to your HVAC system and yourself.**

## My Infinitive Goals
1. Learn Go.
2. Collect HVAC temperature and blower rpm data to chart to visualize HVAC cycle times for no reason other than something to do.
3. Have some geeky fun.

## Getting started

#### Hardware setup
As with the project source developer, all my development is on with Raspberry Pi with Infinitive running on a Pi 4 and my development on a Pi 400. The Pi 4 also runs PiHole and HomeBridge with CPU loads of just a few percent. A Pi Zero would probably do fine, except it has no USB ports (continue reading).

Wiring to the Carrier HVAC employs solid core multi-conductor wire as intended for the purpose, per the referenced GitHub project. In my case, the wire is run adjacent to network and alarm system wires in the basement for a distance of perhaps 20-25 feet up to the ceiling, across, and down to my network equipment.

Found two alternatives while shopping for the RS-485 interface, TTL and USB. The original Infinitive developer used USB and reported on a driver lockup issue and suggested using USB 1 mode. The HASS version author provided instructions for running Infinitive under systemd. systemd is highly recommended, it solves any restart issue with the app or the driver and cleanly manages startup under any condition.
* (eBay) TTL RS-485 interface is:	dlymore TTL Serial Port to RS485 Converter Module, was less than $7.45 with shipping.
* (eBay) USB interface is:	U-485 USB RS485 Serial Port Converter was $8.91 with shipping.

Initially used the RS-485 to TTL adapter wired directly to the serial pins on the GPIO bus which worked as soon as the wires to the HVAC were connected in January 2023. Immediately found the restart issue in the serial driver. The restarts are frequent and found to occur between minutes to after many hours of operation. The code was adapted to these frequent restarts. Much later switched to the USB interface and found it to be stable over days and observed no incidents as were decribed. While it would have permitted a simpler code design, the path taken was educational.

However, after days of using the USB interface, new problems were found. First, the process which purges old log files does not allow the running process to open new log files. So, after the log files are purged there is no log indication of the cause. A programmed forced os.Exit was added to the nightly file processing and chart production code. This lets Systemd do its thing and whatever caused the problem is avoided.
Second, after the first chart is produced just before midnight, the Infinitive.csv file was not rotated and the chart was not created. So, a forced os.Exit solved this problem as well.
The problem was avoided, not identified with this solution, my shame.

There is one use-case for the TTL interface. Infinitive should run quite well on a Pi Zero W. Adding a header to the GPIO bus and using the TTL interface would avoid the need for a micro-USB to USB-A adapter. With systemd, the restarts are not an issue. The work around for the restarts in collecting data allows the dubious TTL interface to work. Might be worth investigating.

Summary, even if you enjoy wiring stuff up and want to use the GPIO pins, don't bother with the TTL option unless you want to use this version on a Pi Zero W (with header).

#### Software
Using go version 1.20.2 linux/arm. Had to fix some import statements in the source code and learn how to setup a Go environment (never saw or used Go before). The enhancements required periodic time based execution. Found [cron v3](https://github.com/robfig/cron) which provides a cron-like time specification. It is used to collect temperature and fan readings at 4 minute intervals. Another cron timer saves daily data to files and then prepares a basic daily chart using [Go E-charts](https://github.com/go-echarts/go-echarts). First pass at charting was pretty simple. Lots is left to learn about the e-chart project. Another timer purges daily data and chart files older than 21 days. Another timer clears the log files 2x per month.
The error log file grew fast with the TTL interface; with the USB interface it contains only the added error messages.

As for the time axis in the charts, have not yet figured out how to set up text format time in the scale. So, the daily data include a date.dayFraction representation of time for the time scale. Just wanted to see a chart more than I wanted to learn e-charts at the time.

There is no axis name on the current charts. Have code which will add the axis name, but it puts it over the chart subtitle. Useless. Have not found suitable example code.

As noted, Infinitive runs under Systemd. Added redirection of output and error files to /var/log/infinitive/. Infinitive is run from /var/lib/infinitive/ with data and chart files also saved there. Data files are in CSV form allowing import into Excel.
The blower RPM scale is the reported fan speed converted to off-low-med-high scale as 0, 34, 66, 100 to use the same y scale as temperarure. Temperature readings and blower RPM readings are sometimes corrupted in transmission, the code cleans up the obvious exteme errors. One day blower RPM will be shown as a right side scale. Changing the time scale to be text date/time is also intended. Axis names are needed as well. So it goes.

The big problems now is building the web user interface assets in order to make UI changes. Not much progress there. Working out bindata and bindata_assetfs differences and how to build the changes and not break everything. Planning on adding a link to the charts from the Infinitive control display once the bindata issue is understood.

To use the executable Pi file, install it in /var/lib/infinitive/ and set it up in systemd. The second link at top has detailed instructions. If you configure systemd to save output and error log files, save them in /var/log/infinitive/ as in the sample infinitive.service file and they will be deleted 2x per month to manage their size.

#### Problems Encountered.
As noted above, the TTL interface is far less stable than the USB interface. Most early development problems would have been avoided had I never tried the TTL interface. I also would have learned less Go.

#### Other Nonsense
With no formal Go experience, I like Go better than many other programming languages I’ve used. I like that Go programs are a single complete executable with no additional support files. I like the sort of C like resemblance and the way objects are referenced. The object-method chaining is kind of neat, but hindered readability at first. Wonder about using the USB RS-485 interface on a Mac and building for macos. Just a thought.

The busy little Pi 4 at work:

Using USB-RS485
![RPi4 - Pihole, Infinitive, HomeBridge](https://github.com/skutoroff/Infinitive-Carrier-HVAC-Enhanced/assets/7796742/815b2c45-3293-4887-b96b-e94e5250f19e)

Using TTL-RS485
![SK_RPi4_InfinitivePiholeHomeBridge](https://github.com/skutoroff/Infinitive-Carrier-HVAC-Enhanced/assets/7796742/19ddfaa0-1728-4202-bb1f-d3513628fa46)

Changed to show the number of restarts, the one restart was a software update.
![2023-06-07_Chart](https://github.com/skutoroff/Infinitive-Carrier-HVAC-Enhanced/assets/7796742/988c611f-15f8-4f63-83ff-301a5c5c855a)
