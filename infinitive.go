package main
	// Ref: https://github.com/acd/infinitive
	//		The inspiration, it was updated late 2023 to eliminate bindata issues
	// Ref: https://github.com/skutoroff/Infinitive-Carrier-HVAC-Enhanced
	//		This development to add data record retention and charting.
	
import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/acd/infinitive/infinity"
	log "github.com/sirupsen/logrus"
	// Added
	"time"
	"strconv"
	"bufio"
	"github.com/robfig/cron/v3"
	"path/filepath"
	"strings"
	"math"
	"io"
	"github.com/go-echarts/go-echarts/v2/charts"
	"github.com/go-echarts/go-echarts/v2/opts"
	"github.com/go-echarts/go-echarts/v2/types"
	"net/http"
)


// Added: Strings used throughout, Version may be changed using -ldflags on build
var	Version			= "development"
var	filePath		= "/var/lib/infinitive/"
var	monthDir		= ""
var	logPath			= "/var/log/infinitive/"
var	linksFile		= "index.html"
var yearFileString	= "Year"
var chartFileSuffix	= "_Infinitive.html"

// Added: api.go external objects, i.e. infinity.BlowerRPM
//		BlowerRPM       uint16
//		HeatSet			uint8
//		CoolSet			uint8
//		CurrentTemp     uint8
//		OutdoorTemp     int8
//		HvacMode		string

// Added: package defs to support periodic write to file
var fileHvacHistory *os.File
var	currentTempPrev	uint8 = 0		// Save previous value for spike removal
var	outdoorTempPrev	int8  = 0		// Save previous value for spike removal
var outTemp			int
var	inTemp			int
var	htmlChartTable	string
var	fileName		string
var	todaysDate		time.Time		// Used to get file age difference
var	todaysYear		int

// Added: Support functions

// timeFromFilePath uses file modified date ttribute
func timeFromFilePath( filename string ) time.Time {
	file, err := os.Stat( filename )
	if (err != nil ) {
		log.Error( "timeFromFilePath os.Stat error on file: " + filename + " ", err )
		return todaysDate
	}
	return file.ModTime()
}	// timeFromFilePath

// return TRUE if the filename is older than today by 2nd argument.
func fileIsTooOld( filename string, limit int ) bool {
	diff	:= todaysDate.Sub(timeFromFilePath(filename)).Hours()/24	// Compute the difference in days from Hours
	return int(diff+math.Copysign(0.5, diff)) > limit -1				// The conversion got a little messy
}	// fileIsTooOld

// Find HTML files and prepare 3 column link table; bool argument controls table only or full html page.
func makeTableHTMLfiles( tableOnly bool, tableFileName string, wayBackDays int ) {
	var files []string

	// Identify the html files, produce table of links, table only or full html page.
	htmlLinks, err := os.OpenFile( tableFileName, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
			log.Error("makeTableHTMLfiles - " + tableFileName + " create Failure.")
	}
	timeStr := time.Now().Format("2006-01-02 15:04:05")
	if !tableOnly {
		htmlLinks.WriteString( "<!-- infinitive.makeTableHTMLfiles(): " + timeStr + " -->\n" )
		htmlLinks.WriteString( "<!DOCTYPE html>\n<html lang=\"en\">\n" )
		htmlLinks.WriteString( "<head>\n<title>HVAC Saved Measurements " + timeStr + "</title>\n" )
		htmlLinks.WriteString( "<style>\n td {\n  text-align: center;\n  }\n table, th, td {\n  border: 1px solid;\n  border-spacing: 5px;\n  border-collapse: collapse;\n }\n</style>\n</head>\n" )
		htmlLinks.WriteString( "<body>\n<h2>HVAC Saved Measurements " + timeStr + "</h2>\n" )
	}
	htmlLinks.WriteString( "<table width=\"720\">\n" )
	err = filepath.Walk( filePath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			log.Error("makeTableHTMLfiles - filepath.Walk error 1." )
			return nil
		}
		// First, only processs html files.
		if !info.IsDir() && filepath.Ext(path) == ".html" {
			// Only add files newer than the criteria for being to old and the Year_yyyy-mmm.html files
			base := filepath.Base( path )
			if base[0:0] == "Y" {				// The few monthy Year charts are listed without considering age.
				files = append(files, path)
			} else {							// The daily chart files
				if !fileIsTooOld(path,wayBackDays) {
					files = append(files, path)
				}
			}
		}
		return nil
	}	) 	// end filepath.Walk()
    if err != nil {
		log.Error("makeTableHTMLfiles - filepath.Walk error 2." )
		return
	} else {
		// Process the filepath list.
		index := 0
		for _, file := range files {
			fileName := file		// still merging code
			length := len(fileName)
			// Only process temperature html files...
			if fileName[length-1] == 'l' && fileName[0]!='h' {
				// make three column table...
				if index % 3 == 0 {
					htmlLinks.WriteString( "  <tr>\n" )
				}
				// For active links using staticServer, omit the leading part of the fileName path.
				//		The index.html file will no longer work without staticServer running, but it will work anywhere on local network.
				// For target, only show the date part of the filename.
				htmlLinks.WriteString( "    <td><a href=\"" + fileName[8:] + "\" target=\"_blank\" rel=\"noopener noreferrer\">" + filepath.Base(fileName) + "</a></td>\n" )
				if index % 3 == 2 {
					htmlLinks.WriteString( " </tr>\n" )
				}
				index++
			}	// file ends with 'l' or starts with 'h' (the files we want.
		}  // end for
		if index % 3 == 1 {
			htmlLinks.WriteString( " </tr>\n" )
		}
	}
	if !tableOnly {
		htmlLinks.WriteString( "</table>\n</body>\n" )
		htmlLinks.WriteString( "</html>\n\n" )
	}
	htmlLinks.Close()
	return
}	// makeTableHTMLfiles

// Next two functions produce html chart of HVAC blower %On history from saved daily html files
//		Find the percent on value searching for "On: ". Code from https://zetcode.com/golang/find-file/
func doOneDailyFile( file string ) int {
	f, err := os.Open(file)
	if err != nil {
		return	-1		// Should not happen
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	line := 1
	for scanner.Scan() {
		if strings.Contains(scanner.Text(), "On: ") {
			index := strings.Index( scanner.Text(), "On: " )
			if index != -1 {
				pcntOn, err := strconv.ParseFloat( strings.Trim( scanner.Text()[index+3:index+10], " " ), 32 )
				if err != nil {
					log.Error("doOneDailyFile conversion error on " + file + ", error: ", err )
				}
				return int( math.Round(pcntOn*10)/10 )		// found it, done with current file (it is always line 19)
				break
			}
		}	// line has "On: "
		line++
	}	// file scanner
	log.Error("doOneDailyFile -no On: value in:" + filepath.Base(file) + ", end line#:", line )
	return -1
}	// doOneDailyFile

// Find html files and extracts the percent on time with the date
func extractPercentFromHTMLfiles( folder string ) {
	var files []string
	var	records	int
	var data[366] int
	var	gapStart int = -1

	records = 0
	dayyr := make( [] int,	366 )
	dayof := make( []opts.LineData, 0)

	err := filepath.Walk(folder, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			log.Error("extractPercentFromHTMLfiles - filepath.Walk() failed 1.", err )
			return nil
		}
		if !info.IsDir() && filepath.Ext(path) == ".html" {
			files = append(files, path)
		}
		return nil
	}	) 	// filepath.Walk()
    if err != nil {
		log.Error("extractPercentFromHTMLfiles - filepath.Walk failed at end", err )
		return
	} else {
		for i := 0; i<366; i++ {									// initialze data array to sawtooth
			dayyr[i]	= i
			data[i]		= -1										// Flag missing data
		}
		// log.Error("extractPercentFromHTMLfile - files in: " + folder )
		for _, file := range files {
			length := len( filepath.Base(file) )					// Short file names will blow up time.Parse
			if filepath.Base(file)==linksFile || filepath.Base(file)[:3]==yearFileString[:3] || filepath.Ext(file)!=".html" || length<10 {
				continue											// Ignore non-html and special case files
			}
			date   := filepath.Base(file)[:10]						// Less dependent on filename structure
			t, err := time.Parse("2006-01-02", date )				// Get date from file name
			if err != nil {
				// There should be few (none?) of these
				log.Error("extractPercentFromHTMLfiles time.Parse() failed. " +  filepath.Base(file) )
			}
			yrday  := t.YearDay()
			if  todaysYear==t.Year() {								// Process current year html files
				data[yrday] = doOneDailyFile( file )				// The current year is processed last by Walk.
				records++
				gapStart = yrday									// gapStart != -1 will mean gap fill starts at gapStart+1
			} else {
				if todaysYear == t.Year()+1 {						// For the previous year, process is same as current year, data will  be oerwriten
					data[yrday] = doOneDailyFile( file )			// The current year is processed last by Walk.
					records++
				} else {
					if todaysYear > t.Year()+1 {					// Older files can be ignored
						continue									// do nothing
					}
				}
			}
		}	// all files
		//	Fill or Overwrite gap between current and prior year. Current is processed last
		if gapStart != -1 && gapStart<350 {
			log.Error("extractPercentFromHTMLfiles - Multi-year fill: ", gapStart )
			for i := gapStart+1; i<gapStart+16;i++ {
				data[i]	= -1										// Fill the gap
			}
		}
		log.Error("extractPercentFromHTMLfiles - Records found: "+  strconv.Itoa(records) )
		for i := 0; i<366; i++ {
			if data[i] != -1 {
				dayof = append( dayof, opts.LineData{ Value: data[i]  } )
			} else {
				dayof = append( dayof, opts.LineData{ Value: nil  } )	// Overwrite line gap
			}
		}	// transfer percent to LineData
		dt := time.Now()
		text := fmt.Sprintf( "Infinitive Vsn: %s, #Found = %d, Date: %04d-%02d-%02d", Version, records, dt.Year(), dt.Month(), dt.Day() )
		Line := charts.NewLine()
		Line.SetGlobalOptions(
			charts.WithInitializationOpts(opts.Initialization{Theme: types.ThemeWesteros}),
			charts.WithTitleOpts(opts.Title{
				Title:    "Infinitive HVAC Pcnt Blower On - Year",
				Subtitle: text,
			}, ),
		)
		// Chart the percent On dayof against dayyr
		Line.SetXAxis( dayyr[0:365] )
		Line.AddSeries("Percent On",  dayof[0:365])
		Line.SetSeriesOptions(charts.WithMarkLineNameTypeItemOpts(opts.MarkLineNameTypeItem{Name: "Maximum", Type: "max"}))
		Line.SetGlobalOptions(
			charts.WithXAxisOpts( opts.XAxis{ AxisLabel: &opts.AxisLabel{Rotate: 45, ShowMinLabel: true, ShowMaxLabel: true, Interval: "0" }, }, ),
			charts.WithXAxisOpts( opts.XAxis{ Name: "Time Year Day",  }, ),
			charts.WithYAxisOpts( opts.YAxis{ Name: "Prcnt On", Type: "value", }, ),
			charts.WithYAxisOpts( opts.YAxis{ Min: 0, Max: 100, }, ),			// apply uniform bounds
		)
		// Render and save the html file...
		fileStr := fmt.Sprintf( yearFileString + "_%04d-%02d.html", dt.Year(), dt.Month() )
		// Chart it
		fHTML, err := os.OpenFile( filePath + fileStr, os.O_CREATE|os.O_APPEND|os.O_RDWR|os.O_TRUNC, 0664 )
		if err == nil {
			// Example Ref: https://github.com/go-echarts/examples/blob/master/examples/boxplot.go
			log.Error("extractPercentFromHTMLfiles - Render to html:  " + fileStr )
			Line.Render(io.MultiWriter(fHTML))
		} else {
			log.Error("extractPercentFromHTMLfiles - Error writing html file: " + fileStr )
		}
		// This works in test app GraphInf, but not here. Cause unknown.
		fHTML.Close()
		err = os.Chmod( fileStr, 0664 )		// as set in OpeFile, still got 0644
	}
}	//extractPercentFromHTMLfiles

// The HVAC data file is opened and closed in different modes at multiple places.
func openDailyFile( timeIs time.Time, fileFlags int, needHeader bool ) (DailyFile *os.File, fileNameIs string) {
	var err error

	fileNameIs = fmt.Sprintf( "%s%4d-%02d-%02d_%s", filePath + monthDir, timeIs.Year(), timeIs.Month(), timeIs.Day(), "Infinitive.csv")
	log.Error( "openDailyFile, Daily:              " + filepath.Base(fileNameIs) )
	DailyFile, err = os.OpenFile(fileNameIs, fileFlags, 0664 )
	if err != nil {
		log.Error( "openDailyFile Create File Failure." )
	}
	if needHeader {
		DailyFile.WriteString( "Date,Time,FracTime,Heat Set,Cool Set,Outdoor Temp,Current Temp,BlowerRPM\n" )
	}
	return
}	// openDailyFile

// Resume ACD
func main() {
	// Added
	var dailyFileName, text	string
	var f64					float64
	var	index				int

	// ACD
	httpPort := flag.Int("httpport", 8080, "HTTP port to listen on")
	serialPort := flag.String("serial", "", "path to serial port")

	flag.Parse()

	if len(*serialPort) == 0 {
		fmt.Print("must provide serial\n")
		flag.PrintDefaults()
		os.Exit(1)
	}

	log.SetLevel(log.ErrorLevel)		// Changed from DebugLevel

	infinityApi, err := infinity.NewApi(context.Background(), *serialPort)
	if err != nil {
		log.Panicf("error opening serial port: %s", err.Error())
	}

	// Added: data collection and charting
	dayf	:= make( [] float32, 2000 )
	inTmp	:= make( [] int,	 2000 )
	outTmp	:= make( [] int,	 2000 )
	motRPM	:= make( [] int,	 2000 )
	outdoorTempPrev = 0
	currentTempPrev = 0

	//	Save the data in a date prefix name file
	dt := time.Now()
	todaysDate	= dt
	todaysYear	= dt.Year()
	monthDir	= fmt.Sprintf( "%04d-%02d/", dt.Year(), dt.Month() )
	fileHvacHistory, dailyFileName = openDailyFile( dt, os.O_APPEND|os.O_CREATE|os.O_WRONLY, true )
	log.Error("Infinitive Start/Restart.")

	// References for periodic execution:
	//		https://pkg.go.dev/github.com/robfig/cron?utm_source=godoc
	//		https://github.com/robfig/cron
	// cron Job 1 - collect data to file every 4 minutes, fix funky values, and start new file at top of the day.
	// cron Job 2 - produce chart and html table before midnight and 2 hours apart from 06:00 to 22:00
	// cron Job 3 - update the Daily html table file and the Year %on time chart.
	// cron job 4 - delete log files 2x per month.

	// Set up cron 1 - 4 minute data collection, fix data, cycle file at top of new day.
	cronJob1 := cron.New(cron.WithSeconds())
	cronJob1.AddFunc("0 */4 * * * *", func () {
		dt = time.Now()
		// Are we at the start of a new day? If so, close yesterdays daily file and open a new one.
		if dt.Hour()==0 && dt.Minute()==0 {
			err = fileHvacHistory.Close()
			if err != nil {
				log.Error("infinitive cron 1 Error closing daily:  " + dailyFileName)
			}
			// Open new file with new date
			fileHvacHistory, dailyFileName = openDailyFile( dt, os.O_APPEND|os.O_CREATE|os.O_WRONLY, true )
		}
		// Consider decimal part calculation with year from 2023, 2023-01-01 is Julian 2459945.5
		frcDay :=  float32(dt.YearDay()) + 4.16667*(float32(dt.Hour()) + float32(dt.Minute())/60.0)/100.0
		// Fix the too frequent 0 or 1 spikes in raw data and range check.
		if ( ( infinity.OutdoorTemp==0 || infinity.OutdoorTemp==1 ) && outdoorTempPrev>10 ) || infinity.OutdoorTemp>125 {
			infinity.OutdoorTemp = outdoorTempPrev
		} else {
			outdoorTempPrev = infinity.OutdoorTemp
		}
		// indoor temp can also be damaged
		if infinity.CurrentTemp<32 || infinity.CurrentTemp>115 {
			infinity.CurrentTemp = currentTempPrev
		} else {
			currentTempPrev = infinity.CurrentTemp
		}
		// Set blower RPM as % where off(0), low(34), med(66), high(100), makes %rpm range match temp range
		if infinity.BlowerRPM < 200 {
			infinity.BlowerRPM = 0
		} else if infinity.BlowerRPM < 550 {
			infinity.BlowerRPM = 34
		} else if infinity.BlowerRPM < 750 {
			infinity.BlowerRPM = 66
		} else {
			infinity.BlowerRPM = 100
		}
		// Future: fix HvacMode, it is sometimes "unknown", but we don't use it.
		outLine := fmt.Sprintf( "%s,%09.4f,%04d,%04d,%04d,%04d,%04d,%s\n", dt.Format("2006-01-02T15:04:05"),
							frcDay, infinity.HeatSet, infinity.CoolSet, infinity.OutdoorTemp, infinity.CurrentTemp, infinity.BlowerRPM, infinity.HvacMode )
		fileHvacHistory.WriteString(outLine)
	} )
	cronJob1.Start()

	// Set up cron 2 for charting daily file at two hour intevals during the day, less at night.
	cronJob2 := cron.New(cron.WithSeconds())
	cronJob2.AddFunc( "2 59 5,7,9,11,13,15,17,19,23 * * *", func() {
		log.Error("Infinitive cron 2 Begins.")
		intervalsRun	:= 0
		intervalsOn		:= 0
		restarts		:= 0
		dt = time.Now()
		// Close, then open new dated Infinitive.csv
		err = fileHvacHistory.Close()
		err = os.Chmod( dailyFileName, 0664 )		// beware file permissions! Or you get 0644.
		if err != nil {
			log.Error("infinitive cron 2 Error closing: " + dailyFileName)
		}
		// Open new file with todays date in name to read captured data.
		fileHvacHistory, err = os.OpenFile( dailyFileName, os.O_RDONLY, 0 )
		if err != nil {
			log.Error("infinitive cron 2 Unable to read daily file: "+dailyFileName)
		}
		// Read and prepare days data for charting
		items1 := make( []opts.LineData, 0 )		// Indoor Temperature
		items2 := make( []opts.LineData, 0 )		// Outdoor Temperature
		items3 := make( []opts.LineData, 0 )		// Blower RPM
		index = 0
		filescan := bufio.NewScanner( fileHvacHistory )
		for filescan.Scan() {
			text = filescan.Text()
			if filescan.Err() != nil {
				log.Error("infinitive cron 2 file Scan read error:" + text )
			}
			if text[0] != 'D' {		// Header lines start with D, skip'em
				f64, err	= strconv.ParseFloat( text[20:29], 32 )
				dayf[index]	= float32(f64)
				// Extract and save the indoor temp, outdoor temps, and blower RPM in slices.
				outTmp[index], err	= strconv.Atoi( text[40:44] )
				inTmp[index], err	= strconv.Atoi( text[45:49] )
				motRPM[index], err	= strconv.Atoi( text[50:54] )
				items1 = append( items1, opts.LineData{ Value: inTmp[index]  } )
				items2 = append( items2, opts.LineData{ Value: outTmp[index] } )
				items3 = append( items3, opts.LineData{ Value: motRPM[index] } )
				// Collect the % active data
				if motRPM[index] > 0 {
					intervalsOn++
				}
				intervalsRun++
				index++
			} else {
				restarts++
			}
		}
		lastData := index-1
		index--
		// If not end of day run, extend time X-axis to expected length.
		if dt.Hour() != 23 {
			base := dayf[index-1] + 0.002777	// bias to match day end time
			for i := index; i<359; i++ {		// 60/4 * 24 = 360
				base += 0.002777				// Next four minute point.
				dayf[i]= base
				index++
			}
		}
		fileHvacHistory.Close()
		log.Error("Infinitive cron 2 Preparing chart: " + filepath.Base(dailyFileName) )
		// echarts referenece: https://github.com/go-echarts/go-echarts
		pcntOn := 100.0 * float32(intervalsOn) / float32(intervalsRun)
		text = fmt.Sprintf("Indoor+Outdoor Temperatue w/Blower RPM from %s, #Restarts: %d, On: %6.1f percent, Vsn: %s %s", dailyFileName, restarts-1, pcntOn, Version, infinity.HvacMode )
		Line := charts.NewLine()
		Line.SetGlobalOptions(
			charts.WithInitializationOpts(opts.Initialization{Theme: types.ThemeWesteros}),
			charts.WithTitleOpts(opts.Title{
				Title:    "Infinitive HVAC Daily Chart",
				Subtitle: text,
			}, ),
		)
		// Chart the Indoor and Outdoor temps (to start). How to use date/time string as time?
		Line.SetXAxis( dayf[0:index])
		Line.AddSeries("Indoor Temp", 	items1[0:lastData])
		Line.AddSeries("Outdoor Temp",	items2[0:lastData])
		Line.SetSeriesOptions(charts.WithMarkLineNameTypeItemOpts(opts.MarkLineNameTypeItem{Name: "Minimum", Type: "min"}))
		Line.SetSeriesOptions(charts.WithMarkLineNameTypeItemOpts(opts.MarkLineNameTypeItem{Name: "Maximum", Type: "max"}))
		Line.AddSeries("Fan RPM%",		items3[0:lastData])
		Line.SetSeriesOptions( charts.WithLineChartOpts( opts.LineChart{Smooth: true} ) )
		// -- In Progress -- Need axis name placement fixed. Y-axis name buried under subtitle, X-axis name to right.
		Line.SetGlobalOptions(
			charts.WithXAxisOpts( opts.XAxis{ AxisLabel: &opts.AxisLabel{Rotate: 45, ShowMinLabel: true, ShowMaxLabel: true, Interval: "0" }, }, ),
			charts.WithXAxisOpts( opts.XAxis{ Name: "Time YearDay.frac",  }, ),				//Type: "time",  }, ),	<<-- Results in diagonal plot
			charts.WithYAxisOpts( opts.YAxis{ Name: "Temp & Blower", Type: "value", }, ), 	//position: "right", }, ),	<<<--wrong.
			charts.WithYAxisOpts( opts.YAxis{ Min: 0, Max: 100, }, ),			// apply uniform bounds
		)
		// Render and save the html file...
		fileStr := fmt.Sprintf( "%s%04d-%02d-%02d"+chartFileSuffix, filePath + monthDir, dt.Year(), dt.Month(), dt.Day() )
		// Chart it all
		fHTML, err := os.OpenFile( fileStr, os.O_CREATE|os.O_APPEND|os.O_RDWR|os.O_TRUNC, 0664 )
		if err == nil {
			// Example Ref: https://github.com/go-echarts/examples/blob/master/examples/boxplot.go
			log.Error("Infinitive cron 2 Render to html:  " + filepath.Base(fileStr) )
			Line.Render(io.MultiWriter(fHTML))
		} else {
			log.Error("Infinitive cron 2 Error html file: " + fileStr )
		}
		fHTML.Close()
		err = os.Chmod( fileStr, 0664 )		// as set in OpenFile, still got 0644
		// Re-open the HVAV history file to write more data, hence append.
		fileHvacHistory, dailyFileName = openDailyFile( dt, os.O_APPEND|os.O_CREATE|os.O_WRONLY, false )
		makeTableHTMLfiles( false, filePath + linksFile, 24 )
	} )
	cronJob2.Start()

	// Set up cron 3 to update the Daily html table file and the Year %on time chart.
	cronJob3 := cron.New(cron.WithSeconds())
	cronJob3.AddFunc( "3 2 0 * * *", func () {
		todaysDate	= dt				// save and update todays date
		todaysYear	= dt.Year()
		// Update the html table file with ~30 days of daily charts and the years chart.
		log.Error("Infinitive cron 3 Prepare the html table of daily charts.")
		makeTableHTMLfiles( false, filePath + linksFile, 24 )
		// Produce Yearly chart daily, destination file will change monthly.
		// Find "On; " in html files to chart extract blower percent on time.
		log.Error("Infinitive cron 3 Prepare Year blower chart percent on time frrom HTML files.")
		extractPercentFromHTMLfiles( filePath )
	} )
	cronJob3.Start()

	// Set up cron 4, Run 1st and 16th of the month to delete log files and exit.
	cronJob4 := cron.New(cron.WithSeconds())
	cronJob4.AddFunc( "4 0 1 1,16 * *", func () {
		log.Error("Infinitive cron 4 Begin log file cycling.")
		// remove log files least they grow unbounded, using shell commands for this was futile.

		logName := logPath + "infinitiveError.log"
		log.Error("infinitive cron 4 Removing Error log file:  " + logName )
		if os.Remove( logName ) != nil {
			log.Error("infinitive cron 4 Removing Error log FAIL:  " + logName )
		}
		logName = logPath + "infinitiveOutput.log"
		log.Error("infinitive cron 4 Removing Output log file: " + logName )
		if os.Remove( logName ) != nil {
			log.Error("infinitive cron 4 Removing Output log FAIL: " + logName )
		}
		// On the 1st day of month, create a month folder
		dt = time.Now()
		if dt.Day() == 1 {
			monthDir = fmt.Sprintf( "%04d-%02d/", dt.Year(), dt.Month() )
			text := filePath + monthDir
			if err := os.Mkdir( text, os.ModePerm ); err == nil {
				log.Error("infinitive cron 4 Create New Month folder created: " + text )
			} else {
				log.Error("infinitive cron 4 Create New Month folder FAILED:  " + text )
			}
		}
		// Log files are not re-opened after this purge. Force an exit and let Systemd sort it out.
		log.Error("Infinitive cron 4 Program Forced Exit after log file purge.")
		os.Exit(1)		// Required so new log files are opened.
	} )
	cronJob4.Start()

	// Start static file server for the charts, asyncrhonous
	log.Error("Infinitive - start FileServer() for Infinitive HVAC charts.")
	go func() {
		// Simple static FileServer
		fs := http.FileServer(http.Dir(filePath[:len(filePath)-1]))			// Remove trailing directory "/"
		http.Handle("/infinitive/", http.StripPrefix("/infinitive/", fs))	// Is this right?
		err:= http.ListenAndServe(":8081", nil)								// localhost:8081/infinitive/index.html
		if err != nil {
			log.Error("Infinitive - Static File Server failed: ListenAndServe. ", err)
		}
   	} ()

	// Start infinitive UI
	log.Error("Infinitive - launchWeserver()   for Infinitive HVAC control.")
	// ACD
	launchWebserver(*httpPort, infinityApi)
}
