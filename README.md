# Infinitive-Enhanced
Based on the project:

Reference:	https://github.com/acd/infinitive

See also:	https://github.com/mww012/hass-infinitive

Start with the infinitive project before further reading here.


The original disclaimer still applies. Maybe more so. (copied from the Infinitive README.md)

## **DISCLAIMER**
**The software and hardware described here interacts with a proprietary system using information gleaned by reverse engineering.  Although it works fine for me, no guarantee or warranty is provided.  Use at your own risk to your HVAC system and yourself.**

## My Infinitive Goals
1. Learn Go.
2. Collect HVAC temperature and fan data for charting to understand HVAC cycle times for no reason other than something to do.
3. Have some geeky fun.

## Getting started

#### Hardware setup
As with the project source developer, all my development is on with Raspberry Pi with Infinitive running on a Pi 4 and my development on a Pi 400. The Pi 4 also runs PiHole and HomeBridge with CPU loads of just a few percent. A Pi Zero would probably do fine.

I use a RS-485 to TTL adapter wired directly to the serial pins on the GPIO bus. It worked as soon as the wires to the HVAC were connected in January 2023. I also have an as yet unused RS-485 to USB adapter, I’m sure it would work just as easily. It was my backup plan.
TTL RS-485 is:	dlymore TTL Serial Port to RS485 Converter Module, was less than $7.45 with shipping from eBay.
USB IF:	U-485 USB RS485 Serial Port Converter was $8.91 with shipping, also found on eBay.

Wiring uses solid core multi-conductor wire as intended for the purpose. Read the referenced GitHub project. The wire is run adjacent to network and alarm system wires in the basement for a distance of perhaps 20-25 feet up to the ceiling and down to my network equipment (see “Problems Encountered).

#### Software
Using go version 1.20.2 linux/arm. Had to fix some import statements in the source code and learn how to setup a Go environment (never saw or used Go before). The enhancements required periodic time based execution. Found [cron v3](https://github.com/robfig/cron) which provides a cron-like time specification. It is used to collect temperature and fan readings at 4 second intervals. Another cron timer saves daily data to files and then prepares a basic daily chart using [Go E-charts](https://github.com/go-echarts/go-echarts). First pass at charting is pretty simple. Lots is left to learn about the e-chart project. A third daily timer runs to purge daily data and chart files older files than 14 days (for now).

As for the time axis in the charts, could not figure out how to set up text format time in the scale. So, the daily data include a date.dayFraction representation of time for the time scale. I wanted to see a chart more than I wanted to learn e-charts at the time.

Having read through the original GitHub project information and some information from web searches, I put Infinitive under Systemd and added redirection of output and error files to /var/log/infinitive/. Infinitive is run from /var/lib/infinitive/ with data and chart files also saved there. Data files are in CSV form allowing import into Excel.

#### Problems Encountered.
After trying to save collected data in arrays (later slices as I learned Go) I discovered that Infinitive crashes, a lot. Systemd did a great job of masking those events. The cause is in the serial driver AFAIK. Had to rewrite code to save data in files and work around the crashes which can occur in intervals from very short to as long as 8 hours observed  between them. It is not clear if household electrical activity is a contributor, the longer durations between crashes do seem to be at night.

#### Plans
Besides improving the charts appearance and adding more time line options, I want to modify the web server to display uptime for my own curiosity and add a table of links to the prepared charts. Currently looking to the web server side of the original source code now that the collection is stable. Working on editing and building the assets side.

#### Other Nonsense
With no formal Go experience, I like Go better than other programming languages I’ve used. I like that Go programs are a single complete executable with no additional support files. I like the sort of C like resemblance and the way objects are referenced. The object-method chaining is kind of neat, but hindered readability at first.

![Image](https://user-images.githubusercontent.com/7796742/235656897-720388c3-07a2-48a0-982a-6f020c1dfb48.JPG)


![Image](https://user-images.githubusercontent.com/7796742/235656510-4a0443b4-1b43-4674-a632-8b629df78702.png)
