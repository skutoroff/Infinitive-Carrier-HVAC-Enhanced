package main

	// Ref: https://github.com/acd/infinitive
	// Ref: https://github.com/elazarl/go-bindata-assetfs
	// Installed to build assets
	//		go get github.com/go-bindata/go-bindata/...
	//		go get github.com/elazarl/go-bindata-assetfs/...
	// Help Ref: https://github.com/inconshreveable/ngrok/issues/181

import (
	"bytes"
	"encoding/binary"
	"flag"
	"fmt"
	"os"
	"time"
	"strconv"
	"bufio"
	"github.com/robfig/cron/v3"
	log "github.com/sirupsen/logrus"
	"io"
	"io/ioutil"
	"github.com/go-echarts/go-echarts/v2/charts"
	"github.com/go-echarts/go-echarts/v2/opts"
	"github.com/go-echarts/go-echarts/v2/types"
	// Below needed for alternative to using bindata_assetfs which cannot be found!
	// "github.com/elazarl/go-bindata-assetfs"
)

type TStatZoneConfig struct {
	CurrentTemp     uint8  `json:"currentTemp"`
	CurrentHumidity uint8  `json:"currentHumidity"`
	OutdoorTemp     uint8  `json:"outdoorTemp"`
	Mode            string `json:"mode"`
	Stage           uint8  `json:"stage"`
	FanMode         string `json:"fanMode"`
	Hold            *bool  `json:"hold"`
	HeatSetpoint    uint8  `json:"heatSetpoint"`
	CoolSetpoint    uint8  `json:"coolSetpoint"`
	RawMode         uint8  `json:"rawMode"`
}

type AirHandler struct {
	BlowerRPM  uint16 `json:"blowerRPM"`
	AirFlowCFM uint16 `json:"airFlowCFM"`
	ElecHeat   bool   `json:"elecHeat"`
}

type HeatPump struct {
	CoilTemp    float32 `json:"coilTemp"`
	OutsideTemp float32 `json:"outsideTemp"`
	Stage       uint8   `json:"stage"`
}

var infinity *InfinityProtocol

// Strings used throughout, may be changed using -ldflags on build if needed
var	Version			= "development"
var	filePath		= "/var/lib/infinitive/"
var	logPath			= "/var/log/infinitive/"
var ChartFileSuffix	= "_Infinitive.html"


// aded: Global defs to support periodic write to file
var fileHvacHistory *os.File
var blowerRPM       uint16
var	currentTemp     uint8
var	currentTempPrev	uint8 = 0		// Save of previous value for spike removeal.
var	outdoorTemp     uint8
var	outdoorTempPrev	uint8 = 0		// Save of previous value for spike removeal.
var	heatSet			uint8
var	coolSet			uint8
var	hvacMode		string
var outTemp			int
var	inTemp			int
var	fanRPM			int
var	index			int
var	htmlChartTable	string
var	fileName		string


// Original Infinitive code with minor changes...
func getConfig() (*TStatZoneConfig, bool) {
	cfg := TStatZoneParams{}
	ok := infinity.ReadTable(devTSTAT, &cfg)
	if !ok {
		return nil, false
	}

	params := TStatCurrentParams{}
	ok = infinity.ReadTable(devTSTAT, &params)
	if !ok {
		return nil, false
	}

	hold := new(bool)
	*hold = cfg.ZoneHold&0x01 == 1

	// Save for periodic cron1 to pick
	currentTemp	= params.Z1CurrentTemp
	inTemp		= int(currentTemp)
	outdoorTemp	= params.OutdoorAirTemp
	outTemp		= int(outdoorTemp)
	heatSet		= cfg.Z1HeatSetpoint
	coolSet		= cfg.Z1CoolSetpoint
	hvacMode	= rawModeToString(params.Mode & 0xf)

	return &TStatZoneConfig{
		CurrentTemp:     params.Z1CurrentTemp,
		CurrentHumidity: params.Z1CurrentHumidity,
		OutdoorTemp:     params.OutdoorAirTemp,
		Mode:            rawModeToString(params.Mode & 0xf),
		Stage:           params.Mode >> 5,
		FanMode:         rawFanModeToString(cfg.Z1FanMode),
		Hold:            hold,
		HeatSetpoint:    cfg.Z1HeatSetpoint,
		CoolSetpoint:    cfg.Z1CoolSetpoint,
		RawMode:         params.Mode,
	}, true
}

func getTstatSettings() (*TStatSettings, bool) {
	tss := TStatSettings{}
	ok := infinity.ReadTable(devTSTAT, &tss)
	if !ok {
		return nil, false
	}

	return &TStatSettings{
		BacklightSetting: tss.BacklightSetting,
		AutoMode:         tss.AutoMode,
		DeadBand:         tss.DeadBand,
		CyclesPerHour:    tss.CyclesPerHour,
		SchedulePeriods:  tss.SchedulePeriods,
		ProgramsEnabled:  tss.ProgramsEnabled,
		TempUnits:        tss.TempUnits,
		DealerName:       tss.DealerName,
		DealerPhone:      tss.DealerPhone,
	}, true
}

func getAirHandler() (AirHandler, bool) {
	b := cache.get("blower")
	tb, ok := b.(*AirHandler)
	if !ok {
		return AirHandler{}, false
	}
	return *tb, true
}

func getHeatPump() (HeatPump, bool) {
	h := cache.get("heatpump")
	th, ok := h.(*HeatPump)
	if !ok {
		return HeatPump{}, false
	}
	return *th, true
}

func statePoller() {
	for {
		c, ok := getConfig()
		if ok {
			cache.update("tstat", c)
		}

		time.Sleep(time.Second * 1)
	}
}

func attachSnoops() {
	// Snoop Heat Pump responses
	infinity.snoopResponse(0x5000, 0x51ff, func(frame *InfinityFrame) {
		data := frame.data[3:]
		heatPump, ok := getHeatPump()
		if ok {
			if bytes.Equal(frame.data[0:3], []byte{0x00, 0x3e, 0x01}) {
				heatPump.CoilTemp = float32(binary.BigEndian.Uint16(data[2:4])) / float32(16)
				heatPump.OutsideTemp = float32(binary.BigEndian.Uint16(data[0:2])) / float32(16)
				log.Debugf("heat pump coil temp is: %f", heatPump.CoilTemp)
				log.Debugf("heat pump outside temp is: %f", heatPump.OutsideTemp)
				cache.update("heatpump", &heatPump)
			} else if bytes.Equal(frame.data[0:3], []byte{0x00, 0x3e, 0x02}) {
				heatPump.Stage = data[0] >> 1
				log.Debugf("HP stage is: %d", heatPump.Stage)
				cache.update("heatpump", &heatPump)
			}
		}
	})

	// Snoop Air Handler responses
	infinity.snoopResponse(0x4000, 0x42ff, func(frame *InfinityFrame) {
		data := frame.data[3:]
		airHandler, ok := getAirHandler()
		if ok {
			if bytes.Equal(frame.data[0:3], []byte{0x00, 0x03, 0x06}) {
				airHandler.BlowerRPM = binary.BigEndian.Uint16(data[1:5])
				log.Debugf("blower RPM is: %d", airHandler.BlowerRPM)
				cache.update("blower", &airHandler)
				blowerRPM = airHandler.BlowerRPM		// added
				fanRPM = int(blowerRPM)					// added
			} else if bytes.Equal(frame.data[0:3], []byte{0x00, 0x03, 0x16}) {
				airHandler.AirFlowCFM = binary.BigEndian.Uint16(data[4:8])
				airHandler.ElecHeat = data[0]&0x03 != 0
				log.Debugf("air flow CFM is: %d", airHandler.AirFlowCFM)
				cache.update("blower", &airHandler)
			}
		}
	})

}

// Find HTML files and prepare 3 column link table; bool argument controls table only or full html page.
func makeTableHTMLfiles( tableOnly bool ) {
	// Identify the html files, produce table of links, table only or full html page.
	htmlLinks, err := os.OpenFile(filePath+"htmlLinks.html", os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
			log.Error("infinitive.makeTableHTMLfiles - htmlLinks.html Create Failure.")
	}
	if !tableOnly {
		timeStr := time.Now().Format("2006-01-02 15:04:05")
		htmlLinks.WriteString( "<!-- infinitive.makeTableHTMLfiles(): " + timeStr + " -->\n" )
		htmlLinks.WriteString( "<!DOCTYPE html>\n<html lang=\"en\">\n" )
		htmlLinks.WriteString( "<head>\n<title>HVAC Saved Measurements " + timeStr + "</title>\n" )
		htmlLinks.WriteString( "<style>\n td {\n  text-align: center;\n  }\n table, th, td {\n  border: 1px solid;\n  border-spacing: 5px;\n  border-collapse: collapse;\n }\n</style>\n</head>\n" )
		htmlLinks.WriteString( "<body>\n<h2>HVAC Saved Measurements " + timeStr + "</h2>\n" )
	}
	htmlLinks.WriteString( "<table width=\"500\">\n" )
	files, err := ioutil.ReadDir( filePath[0:len(filePath)-1] )  // does not want trailing /
	if err != nil {
		log.Error("infinitive.makeTableHTMLfiles - Dirctory Read Error.")
	} else {
		index = 0
		for _, file := range files {
			fileName := file.Name()
			length := len(fileName)
			// Only process temperature html files...
			if fileName[length-1] == 'l' && fileName[0]!='h' {
				// make three column table...
				if index % 3 == 0 {
					htmlLinks.WriteString( "  <tr>\n" )
				}
				// Only show the date part of the filename.
				htmlLinks.WriteString( "    <td><a href=\"" + filePath + fileName + "\" target=\"_blank\" rel=\"noopener noreferrer\">" + fileName[0:10] + "</a></td>\n" )
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
	htmlLinks.WriteString( "</table>\n" )
	if !tableOnly {
		htmlLinks.WriteString( "</body>\n</html>\n\n" )
	}
	htmlLinks.Close()
	return
}

// The HVAC data file is opened and closed in different modes at multiple places.
func OpenDailyFile( timeIs time.Time, fileFlags int, needHeader bool ) (DailyFile *os.File, fileNameIs string) {
	var err error

	fileNameIs = fmt.Sprintf( "%s%4d-%02d-%02d_%s", filePath, timeIs.Year(), timeIs.Month(), timeIs.Day(), "Infinitive.csv")
	log.Error( "infinitive.OpenDailyFile, Daily:   " + fileNameIs )
	DailyFile, err = os.OpenFile(fileNameIs, fileFlags, 0664 )
	if err != nil {
		log.Error( "Infinitive OpenDailyFile Create File Failure." )
	}
	if needHeader {
		DailyFile.WriteString( "Date,Time,FracTime,Heat Set,Cool Set,Outdoor Temp,Current Temp,blowerRPM\n" )
	}
	return
}

func main() {
	var dailyFileName, text	string
	var f64					float64

	httpPort := flag.Int("httpport", 8080, "HTTP port to listen on")
	serialPort := flag.String("serial", "", "path to serial port")

	flag.Parse()

	if len(*serialPort) == 0 {
		log.Info("must provide serial\n")
		flag.PrintDefaults()
		os.Exit(1)
	}

	log.SetLevel(log.ErrorLevel)		// was log.DebugLevel

	infinity = &InfinityProtocol{device: *serialPort}
	airHandler := new(AirHandler)
	heatPump   := new(HeatPump)
	cache.update("blower", airHandler)
	cache.update("heatpump", heatPump)
	attachSnoops()
	err := infinity.Open()
	if err != nil {
		log.Panicf("error opening serial port: %s", err.Error())
	}

	// added for data collection and charting
	dayf	:= make( [] float32, 2000 )
	inTmp	:= make( [] int,	 2000 )
	outTmp	:= make( [] int,	 2000 )
	motRPM	:= make( [] int,	 2000 )
	outdoorTempPrev = 0
	currentTempPrev = 0

	//	Save the data in a date prefix name file
	dt := time.Now()
	fileHvacHistory, dailyFileName = OpenDailyFile( dt, os.O_APPEND|os.O_CREATE|os.O_WRONLY, true )
	log.Error("Infinitive Start/Restart.")

	// References for periodic execution:
	//		https://pkg.go.dev/github.com/robfig/cron?utm_source=godoc
	//		https://github.com/robfig/cron
	// cron Job 1 - collect data to file every 4 minutes, fix funky values, and start new file at top of the day.
	// cron Job 2 - produce chart and html table before midnight and 2 hours apart from 06:00 to 22:00
	// cron Job 3 - delete csv and html files after 28 days.
	// cron job 4 - delete log files 2x per month.

	// Set up cron 1 - 4 minute data collection, fix data, cycle file at top of new day.
	cronJob1 := cron.New(cron.WithSeconds())
	cronJob1.AddFunc("0 */4 * * * *", func () {
		dt = time.Now()
		// Are we at the start of a new day? If so, close yesterdays daily file and open a new one.
		if dt.Hour()==0 && dt.Minute()==0 {
			err = fileHvacHistory.Close()
			if err != nil {
				log.Error("infinitive cron 2 Error closing daily:  " + dailyFileName)
			}
			// Open new file with new date
			fileHvacHistory, dailyFileName = OpenDailyFile( dt, os.O_APPEND|os.O_CREATE|os.O_WRONLY, true )
		}
		// Consider decimal part calculation with year from 2023, 2023-01-01 is Julian 2459945.5
		frcDay :=  float32(dt.YearDay()) + 4.16667*(float32(dt.Hour()) + float32(dt.Minute())/60.0)/100.0
		// Fix the too frequent 0 or 1 spikes in raw data and range check.
		if ( ( outdoorTemp==0 || outdoorTemp==1 ) && outdoorTempPrev>10 ) || outdoorTemp>130 {
			outdoorTemp = outdoorTempPrev
		} else {
			outdoorTempPrev = outdoorTemp
		}
		// indoor temp can also be damaged
		if currentTemp<32 || currentTemp>115 {
			currentTemp = currentTempPrev
		} else {
			currentTempPrev = currentTemp
		}
		// Set blower RPM as % where off(0), low(34), med(66), high(100), makes %rpm range match temp range
		if blowerRPM < 200 {
			blowerRPM = 0
		} else if blowerRPM < 550 {
			blowerRPM = 34
		} else if blowerRPM < 750 {
			blowerRPM = 66
		} else {
			blowerRPM = 100
		}
		// Future: fix hvacMode, it is sometimes "unknown", but we don't use it.
		outLine := fmt.Sprintf( "%s,%09.4f,%04d,%04d,%04d,%04d,%04d,%s\n", dt.Format("2006-01-02T15:04:05"),
							frcDay, heatSet, coolSet, outdoorTemp, currentTemp, blowerRPM, hvacMode )
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
		log.Error("Infinitive cron 2 Preparing chart: " + dailyFileName)
		// echarts referenece: https://github.com/go-echarts/go-echarts
		pcntOn := 100.0 * float32(intervalsOn) / float32(intervalsRun)
		text = fmt.Sprintf("Indoor+Outdoor Temperatue w/Blower RPM from %s, #Restarts: %d, On: %6.1f percent, Vsn: %s", dailyFileName, restarts-1, pcntOn, Version )
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
		)
		// Render and save the html file...
		fileStr := fmt.Sprintf( "%s%04d-%02d-%02d"+ChartFileSuffix, filePath, dt.Year(), dt.Month(), dt.Day() )
		// Chart it all
		fHTML, err := os.OpenFile( fileStr, os.O_CREATE|os.O_APPEND|os.O_RDWR|os.O_TRUNC, 0664 )
		if err == nil {
			// Example Ref: https://github.com/go-echarts/examples/blob/master/examples/boxplot.go
			log.Error("Infinitive cron 2 Render to html:  " + fileStr )
			Line.Render(io.MultiWriter(fHTML))
		} else {
			log.Error("Infinitive cron 2 Error html file: " + fileStr )
		}
		fHTML.Close()
		err = os.Chmod( fileStr, 0664 )		// as set in OpeFile, still got 0644
		// Re-open the HVAV history file to write more data, hence append.
		fileHvacHistory, dailyFileName = OpenDailyFile( dt, os.O_APPEND|os.O_CREATE|os.O_WRONLY, false )
		makeTableHTMLfiles( false )
	} )
	cronJob2.Start()

	// Set up cron 3 to purge old daily csv & html files
	// Note: Tried variations of shell exec for this, none worked.
	cronJob3 := cron.New(cron.WithSeconds())
	cronJob3.AddFunc( "3 2 0 * * *", func () {
		// Code assumes file order is old 2 new. It has been.
		log.Error("Infinitive cron 3 Begin purge old files.")
		count := 0
		nowDayYear := time.Now().YearDay()
		files, err := ioutil.ReadDir( filePath[0:len(filePath)-1] )  // does not want trailing /
		if err != nil {
			log.Error( "Infinitive cron 3 Directory read error: " + filePath )
			log.Error(err)
		} else {
			for _, file := range files {
				fileName := file.Name()
				length := len(fileName)
				fullName := filePath + fileName
				// Process csv & html files...
				if fileName[0]=='2' && (fileName[length-1] == 'v' || fileName[length-1] == 'l') {
					fFile, err := os.Stat( fullName )
					if err == nil {
						dayofYear := fFile.ModTime().YearDay()
						if nowDayYear - dayofYear > 29 {
							count++
							if os.Remove( fullName ) != nil {
								log.Error( "Infinitive cron 3 Error removing:  " + fullName )
							} else {
								log.Error( "Infinitive cron 3 Removed file:    " + fullName )
							}
						}
						if count > 3 { break }	// Limit number of deletes (expect 3 per day).
					} else {
						log.Error( "Infinitive cron 3, can't os.Stat: " + fullName )
					}	// os.Stat issue
				} // fileName is match
			}  // for...
		}
		makeTableHTMLfiles( false )
	} )
	cronJob3.Start()

	// Set up cron 4 to delete log files 1st and 16th of the month
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
		// Log files are not re-opened after this purge. Force an exit and let Systmd sort it out.
		log.Error("Infinitive cron 4 Program Forced Exit after log file purge.")
		os.Exit(1)		// Required so new log files are opened.
	} )
	cronJob4.Start()

	// Code using: https://github.com/elazarl/go-bindata-assetfs
	go statePoller()
	webserver(*httpPort)
	// Below said to be needed for alternative to bindata_assetfs, remains unsolved.
	//http.Handle("/", http.FileServer(
	//	&assetfs.AssetFS{Asset: Asset, AssetDir: AssetDir, AssetInfo: AssetInfo, Prefix: "data"}))
}
