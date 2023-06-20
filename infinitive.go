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
	// Below needed for alternative to using bindata_assetfs which cannot be found!
	// "github.com/elazarl/go-bindata-assetfs"

	"github.com/go-echarts/go-echarts/v2/charts"
	"github.com/go-echarts/go-echarts/v2/opts"
	"github.com/go-echarts/go-echarts/v2/types"
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
var	fileName		= "Infinitive.csv"
var	logPath			= "/var/log/infinitive/"
var ChartFileSuffix	= "_Chart.html"


// aded: Global defs to support periodic write to file
var fileHvacHistory *os.File
var blowerRPM       uint16
var	currentTemp     uint8
var	outdoorTemp     uint8
var	heatSet			uint8
var	coolSet			uint8
var	hvacMode		string
var outTemp			int
var	inTemp			int
var	fanRPM			int
var	index			int
var	htmlChartTable	string


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

// Function to find HTML files and prepare table of links, bool argument controls table only or full html page
func makeTableHTMLfiles( tableOnly bool ) {
	// Identify the html files, produce 2 column html table of links
	htmlLinks, err := os.OpenFile(filePath+"htmlLinks.html", os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
			log.Error("infinitive.makeTableHTMLfiles:htmlFile Create Failure.")
	}
	if !tableOnly {
		timeStr := time.Now().Format("2006-01-02 15:04:05")
		htmlLinks.WriteString( "<!-- infinitive.makeTableHTMLfiles(): " + timeStr + " -->\n" )
		htmlLinks.WriteString( "<!DOCTYPE html>\n<html lang=\"en\">\n" )
		htmlLinks.WriteString( "<head>\n<title>HVAC Saved Measuremnts " + timeStr + "</title>\n" )
		htmlLinks.WriteString( "<style>\n td {\n  text-align: center;\n  }\n table, th, td {\n  border: 1px solid;\n  border-spacing: 5px;\n  border-collapse: collapse;\n }\n</style>\n</head>\n" )
		htmlLinks.WriteString( "<body>\n<h2>HVAC Saved Measuremnts " + timeStr + "</h2>\n" )
	}
	htmlLinks.WriteString( "<table width=\"500\">\n" )
	files, err := ioutil.ReadDir( filePath[0:len(filePath)-1] )  // does not want trailing /
	if err != nil {
		log.Error("infinitive.makeTableHTMLfiles() - file dirctory read error.")
		log.Error(err)
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
				htmlLinks.WriteString( "    <td><a href=\"" + filePath + fileName + "\" target=\"_blank\" rel=\"noopener noreferrer\">" + fileName[0:10] + "</a></td>\n" )
				if index % 3 == 2 {
					htmlLinks.WriteString( " </tr>\n" )
				}
				index++
			}
		}
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

func main() {
	var HeaderString	= "Date,Time,FracTime,Heat Set,Cool Set,Outdoor Temp,Current Temp,blowerRPM\n"
	var dailyFileName, s2	string
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
	dt := time.Now()
	//	Save the data in a file, observed crashing requires charting from file
	fileHvacHistory, err = os.OpenFile(filePath+fileName, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0664 )
	if err != nil {
			log.Error("Infinitive Data File Open Failure.")
	}
	fileHvacHistory.WriteString( HeaderString )
	log.Error("Infinitive Start/Restart.")

	// References for periodic execution:
	//		https://pkg.go.dev/github.com/robfig/cron?utm_source=godoc
	//		https://github.com/robfig/cron
	// cron Job 1 - every 4 minutes - collect to Infinitive.csv
	// cron Job 2 - after last data of the day - close, rename, open new Infinitive.csv, & produce html from last file
	// cron Job 3 - purge daily files after 28 days
	// cron job 4 - delete log files 2x per month
	// Set up cron 1 for 4 minute data collection
	cronJob1 := cron.New(cron.WithSeconds())
	cronJob1.AddFunc("0 */4 * * * *", func () {
		dt = time.Now()
		// Consider including years fro 2023 in calculaton, 2023-05-01 is Julian 2460065
		frcDay :=  float32(dt.YearDay()) + 4.16667*(float32(dt.Hour()) + float32(dt.Minute())/60.0)/100.0
		s1 := fmt.Sprintf( "%s,%09.4f,%04d,%04d,%04d,%04d,%04d,%s\n", dt.Format("2006-01-02T15:04:05"),
							frcDay,heatSet, coolSet, outdoorTemp, currentTemp, blowerRPM, hvacMode )
		fileHvacHistory.WriteString(s1)
	} )
	cronJob1.Start()

	// Set up cron 2 for daily file save after last collection, data clean up, and charting. Now with forced exit!
	cronJob2 := cron.New(cron.WithSeconds())
	cronJob2.AddFunc( "2 59 23 * * *", func() {
		dt = time.Now()
		// Close, rename, open new Infinitive.csv
		log.Error("Infinitive cron 2 Begins.")
		err = fileHvacHistory.Close()
		if err != nil {
			log.Error("infinitive cron 2 Error closing: " + filePath+fileName)
			os.Exit(0)
		}
		dailyFileName = fmt.Sprintf( "%s%4d-%02d-%02d_%s", filePath, dt.Year(), dt.Month(), dt.Day(), fileName)
		log.Error("infinitive cron 2 Daily HVAC data file: " + dailyFileName)
		err = os.Rename(filePath+fileName,dailyFileName)
		if err != nil {
			log.Error("infinitive cron 2 Unable to rename old file: "+filePath+fileName+" to: "+dailyFileName )
			os.Exit(0)
		}
		// Reopen/Open new Infinitive.csv
		fileHvacHistory, err = os.OpenFile(filePath+fileName, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0664 )
		if err != nil {
			log.Error("infinitive cron Job 2 Error on reopen of: "+filePath+fileName)
			os.Exit(0)
		}
		fileHvacHistory.WriteString( HeaderString )
		// Open the renamed file to read captured data
		fileDaily, err := os.OpenFile( dailyFileName, os.O_RDONLY, 0 )
		if err != nil {
			log.Error("infinitive cron 2 Unable to open daily file for read: "+dailyFileName)
			os.Exit(0)
		}
		// Read and prepare days data for charting
		items1 := make( []opts.LineData, 0 )		// Indoor Temperature
		items2 := make( []opts.LineData, 0 )		// Outdoor Temperature
		items3 := make( []opts.LineData, 0 )		// Blower RPM
		index = 0
		restarts := 0
		filescan := bufio.NewScanner( fileDaily )
		for filescan.Scan() {
			s2 = filescan.Text()
			if filescan.Err() != nil {
				log.Error("infinitive cron 2 file Scan read error:" + s2 )
			}
			if s2[0] != 'D' {		// Header lines start with D, skip'em
				f64, err	= strconv.ParseFloat( s2[20:29], 32 )
				dayf[index]	= float32(f64)
				// Extract and save the indoor and outdoor temps in the slices (not yet used)
				outTmp[index], err	= strconv.Atoi( s2[40:44] )
				// fix for the too frequent 0 spikes in raw data
				if outTmp[index]==0 || outTmp[index]>130 {		// outTmp could be 0, but likely an error
					outTmp[index] = outTmp[index-1]				// worry about index==0 later
				}
				if outTmp[index]==1 && outTmp[index-1]>25 { 	// we get down spikes to 1 as well as 0
					outTmp[index] = outTmp[index-1]				// again, worry about inedx==0 later
				}
				inTmp[index], err	= strconv.Atoi( s2[45:49] )
				if inTmp[index]==0 || inTmp[index]>110 {
					inTmp[index] = inTmp[index-1]				// worry about index==0 later
				}
				motRPM[index], err = strconv.Atoi( s2[50:54] )
				// Set low-med-Hi ranges to improve chart, for now %rpm range matches temp degree range
				if motRPM[index] < 200 {
					motRPM[index] = 0
				} else if motRPM[index] < 550 {
					motRPM[index] = 34
				} else if motRPM[index] < 750 {
					motRPM[index] = 66
				} else {
					motRPM[index] = 100
				}
				items1 = append( items1, opts.LineData{ Value: inTmp[index]  } )
				items2 = append( items2, opts.LineData{ Value: outTmp[index] } )
				items3 = append( items3, opts.LineData{ Value: motRPM[index] } )
				index++
			} else {
				restarts++
			}
		}
		index--
		fileDaily.Close()
		log.Error("Infinitive cron 2 Preparing chart from: " + dailyFileName)
		// echarts referenece: https://github.com/go-echarts/go-echarts
		s2 = fmt.Sprintf("Indoor+Outdoor Temperatue w/Blower RPM from %s, #Restarts: %d, Vsn: %s", dailyFileName, restarts-1, Version )
		Line := charts.NewLine()
		Line.SetGlobalOptions(
			charts.WithInitializationOpts(opts.Initialization{Theme: types.ThemeWesteros}),
			charts.WithTitleOpts(opts.Title{
				Title:    "Infinitive HVAC Daily Chart",
				Subtitle: s2,
			}, ),
		)
		// Chart the Indoor and Outdoor temps (to start). How to use date/time string as time?
		Line.SetXAxis( dayf[0:index])
		Line.AddSeries("Indoor Temp", 	items1[0:index])
		Line.AddSeries("Outdoor Temp",	items2[0:index])
		Line.SetSeriesOptions(charts.WithMarkLineNameTypeItemOpts(opts.MarkLineNameTypeItem{Name: "Minimum", Type: "min"}))
		Line.SetSeriesOptions(charts.WithMarkLineNameTypeItemOpts(opts.MarkLineNameTypeItem{Name: "Maximum", Type: "max"}))
		Line.AddSeries("Fan RPM%",		items3[0:index])
		Line.SetSeriesOptions(charts.WithLineChartOpts(opts.LineChart{Smooth: true}))
		// Render and save the html file...
		fileStr := fmt.Sprintf( "%s%04d-%02d-%02d%s", filePath, dt.Year(), dt.Month(), dt.Day(), ChartFileSuffix )
		// Chart it all
		fHTML, err := os.OpenFile( fileStr, os.O_CREATE|os.O_APPEND|os.O_RDWR|os.O_TRUNC, 0664 )
		if err == nil {
			// Example Ref: https://github.com/go-echarts/examples/blob/master/examples/boxplot.go
			log.Error("Infinitive cron 2 Begin rendering html file: " + fileStr )
			Line.Render(io.MultiWriter(fHTML))
		} else {
			log.Error("Infinitive cron 2 Error creating html chart: " + fileStr )
		}
		fHTML.Close()
		err = os.Chmod( fileStr, 0664 )		// as set in OpeFile, still got 0644
	} )
	cronJob2.Start()

	// Set up cron 3 to purge old daily csv & html files
	// Tried variations of shell exec does not work.
	cronJob3 := cron.New(cron.WithSeconds())
	cronJob3.AddFunc( "3 2 0 * * *", func () {
		// Limitations as code elaborated: assumes file order is old 2 new.
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
						if nowDayYear - dayofYear > 28 {
							count++
							if os.Remove( fullName ) != nil {
								log.Error( "Infinitive cron 3 Error removing: " + fullName )
							} else {
								log.Error( "Infinitive cron 3 Removed file:   " + fullName )
							}
						}
						if count > 3 { break }	// Limit number of deletes (expect 2 per day).
					} else {
						log.Error( "Infinitive cron 3, can't os.Stat file: " + fullName )
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
		// remove log files least they grow unbounded, using shell commands for this was futile
		logName := logPath + "infinitiveError.log"
		log.Error("infinitive cron 4 Try removing Error log file:    " + logName )
		if os.Remove( logName ) != nil {
			log.Error("infinitive cron 4 Error removing Error log file:  " + logName )
		}
		logName = logPath + "infinitiveOutput.log"
		log.Error("infinitive cron 4 Try removing Output log file:   " + logName )
		if os.Remove( logName ) != nil {
			log.Error("infinitive cron 4 Error removing Output log file: " + logName )
		}
		// Log files are not re-opened after this purge. Force an exit and let Systmd sort it out.
		log.Error("Infinitive cron 4 Program Forced Exit after log file purge.")
		os.Exit(1)
	} )
	cronJob4.Start()

	// Code using: https://github.com/elazarl/go-bindata-assetfs
	go statePoller()
	webserver(*httpPort)
	// Below said to be needed for alternative to bindata_assetfs, remains unsolved.
	//http.Handle("/", http.FileServer(
	//	&assetfs.AssetFS{Asset: Asset, AssetDir: AssetDir, AssetInfo: AssetInfo, Prefix: "data"}))
}
