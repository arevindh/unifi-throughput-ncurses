package main

import (
	"fmt"
	"strings"
	"net/http"
	"io/ioutil"
	"encoding/json"
	"time"
	"github.com/BurntSushi/toml"
	"math"
	gc "github.com/rthornton128/goncurses"
	"strconv"
	"net/http/cookiejar"
	"os"
	"flag"
	"os/user"
	"errors"
)

type Configuration struct {
	Username      string
	Password      string
	Site          string
	Url           string
	UploadColor   string
	DownloadColor string
	UseBars       bool
}

const DOWNLOAD_COLOR = 1
const UPLOAD_COLOR = 2
const DOWNLOAD_TEXT_COLOR = 3
const UPLOAD_TEXT_COLOR = 4
const BLACK = 5
const ERROR_COLOR = 6
const DEFAULT_CONFIG_FOLDER = "/.config/unifi-throughput"
const DEFAULT_CONFIG_PATH = DEFAULT_CONFIG_FOLDER + "/config.toml"
const BAR_WIDTH = 10
const CIRCLE_WIDTH = 2

//passed by compiler
var VERSION string

func main() {

	externalConfig := flag.String("config", GetDefaultConfigPath(), "External configuration file location")
	showVersion := flag.Bool("version", false, "Show version")
	createConfig := flag.Bool("create-config", false, "Create the default config file "+GetDefaultConfigPath()+" THIS WILL OVERWRITE YOUR CURRENT CONFIG AT THE DEFAULT LOCATION")
	flag.Parse()

	//using external config, skipping the argument switch

	if *showVersion {
		fmt.Println("Unifi Throughput", VERSION)
	} else if *createConfig {
		CreateDefaultConfig()
	} else {
		StartApp(*externalConfig)
	}

}

// Gets the default config location
func GetDefaultConfigPath() string {
	usr, _ := user.Current()
	return usr.HomeDir + DEFAULT_CONFIG_PATH
}

func GetDefaultConfigFolder() string {
	usr, _ := user.Current()
	return usr.HomeDir + DEFAULT_CONFIG_FOLDER

}

func CreateDefaultConfig() {

	os.MkdirAll(GetDefaultConfigFolder(), os.ModePerm)

	config := []byte("#Controller URL, do not add tailing /\n" +
		"url=\"https://demo.ubnt.com\"\n" +
		"# Name of the site" +
		"\nsite = \"default\"\n\n" +
		"#credentials to login to the controller\n" +
		"username = \"superadmin\"\n" +
		"password =\"\"\n\n" +
		"# Colors for the bars and text options: blue, green, yellow, magenta, cyan, red, white\n" +
		"UploadColor = \"blue\"\n" +
		"DownloadColor = \"cyan\"\n\n" +
		"# Usebars = true to use bar instead of circles, in case you face display issues\n" +
		"UseBars = false")
	err := ioutil.WriteFile(GetDefaultConfigPath(), config, 0644)
	if err != nil {
		fmt.Println(err)
	} else {
		fmt.Println("Config created at", GetDefaultConfigPath())
	}
}

func MapColor(colorText string) int16 {
	switch colorText {
	case "blue":
		return gc.C_BLUE
	case "red":
		return gc.C_RED
	case "green":
		return gc.C_GREEN
	case "yellow":
		return gc.C_YELLOW
	case "magenta":
		return gc.C_MAGENTA
	case "cyan":
		return gc.C_CYAN
	default:
		return gc.C_WHITE
	}

}

//Runs the actual Application
func StartApp(configFile string) {
	config := openConfig(configFile)

	cookieJar, _ := cookiejar.New(nil);
	client := &http.Client{
		Jar: cookieJar,
	}

	stdscr, _ := gc.Init()
	defer gc.End()

	if !gc.HasColors() {
		panic("Example requires a colour capable terminal")
	}

	// Must be called after Init but before using any colour related functions
	if err := gc.StartColor(); err != nil {
		panic(err)
	}

	maxY, maxX := stdscr.MaxYX()

	maxHeight := int(float64(maxY) * 1)
	maxWidth := int(float64(maxX) * 0.1)

	fmt.Println("maxx", maxX, "maxy", maxY)

	uploadColor := MapColor(config.UploadColor)
	downloadColor := MapColor(config.DownloadColor)

	if err := gc.InitPair(DOWNLOAD_COLOR, downloadColor, downloadColor); err != nil {
		panic(err)
	}

	if err := gc.InitPair(UPLOAD_COLOR, uploadColor, uploadColor); err != nil {
		panic(err)
	}

	if err := gc.InitPair(UPLOAD_TEXT_COLOR, uploadColor, gc.C_BLACK); err != nil {
		panic(err)
	}

	if err := gc.InitPair(DOWNLOAD_TEXT_COLOR, downloadColor, gc.C_BLACK); err != nil {
		panic(err)
	}
	if err := gc.InitPair(BLACK, gc.C_BLACK, gc.C_BLACK); err != nil {
		panic(err)
	}

	if err := gc.InitPair(ERROR_COLOR, gc.C_WHITE, gc.C_BLACK); err != nil {
		panic(err)
	}

	sideMargin := 3
	upload, err := gc.NewWindow(maxHeight, maxWidth, 0, sideMargin)
	if (err != nil ) {
		panic(err)
	}

	download, err := gc.NewWindow(maxHeight, maxWidth, 0, maxX-maxWidth-sideMargin)
	if (err != nil ) {
		panic(err)
	}

	stdscr.MovePrint(maxY, maxX/2, "Latency")


	stdscr.Refresh()

	go GetData(&config, client, stdscr, upload, download)

	loop := true
	for loop  {
		switch char := stdscr.GetChar(); char{
		default:
			config.UseBars = !config.UseBars
		}
	}
}

// Starts the actual logic of the application
func GetData(config *Configuration, client *http.Client, screen *gc.Window, uploadBar *gc.Window, downloadBar *gc.Window) {
	var maxValue float64 = 0

	if err := login(config.Url, config.Username, config.Password, client); err != nil {
		//panic(err)
		ShowErrorScreen(screen, err)
	}

	for {
		latency, upload, download, err := getInfo(config.Url, config.Site, client)
		if err != nil {
			//trying to login again, that could be a cookie expired...
			login(config.Url, config.Username, config.Password, client)
			ShowErrorScreen(screen, err)
		} else {

			maxValue = math.Max(upload, maxValue)
			maxValue = math.Max(download, maxValue)

			DisplayData(latency, upload, download, maxValue, screen, uploadBar, downloadBar, config.UseBars);
		}
		time.Sleep(3 * time.Second)
	}
}

// Display stuff on ncurses
func DisplayData(latency float64, upload float64, download float64, maxValue float64, screen *gc.Window, uploadBar *gc.Window, downloadBar *gc.Window, useBars bool) {
	//keeping the max value

	//getting the speed in mbps
	readableUpload := bytesToMebibit(upload)
	readableDownload := bytesToMebibit(download)

	maxUploadPercent := (upload / maxValue) * 100
	maxDownloadPercent := (download / maxValue) * 100

	maxY, maxX := screen.MaxYX()

	latencyText := "Latency: " + strconv.FormatFloat(latency, 'f', 0, 64) + "ms"
	speedText := "Speeds (mbps)"

	// If we have more than 4 digits, we convert to gbps, we have time to see up to tbps //todo: remind me in 20 years
	if readableDownload >= 1000 || readableUpload >= 1000 {
		readableDownload /= 1000
		readableUpload /= 1000
		speedText = "Speeds (gbps)"
	}

	uploadText := "^" + strconv.FormatFloat(readableUpload, 'f', 2, 64)
	downloadText := "-" + strconv.FormatFloat(readableDownload, 'f', 2, 64)

	screen.Erase()
	screen.Refresh()

	if useBars {
		UpdateBar(screen, maxUploadPercent, 1, UPLOAD_COLOR)

		UpdateBar(screen, maxDownloadPercent, maxX-BAR_WIDTH-1, DOWNLOAD_COLOR)
	} else {
		uploadAngle := int((maxUploadPercent / 100) * 180)
		downloadAngle := int(( (100 - maxDownloadPercent) / 100) * 180)

		radius := (math.Min(float64(maxY), float64(maxX)) - 2) / 2
		screen.ColorOn(UPLOAD_COLOR)
		DrawCircle(maxY/2, maxX/2-1, radius, 90, 90+uploadAngle, CIRCLE_WIDTH, screen)
		screen.ColorOff(UPLOAD_COLOR)

		screen.ColorOn(DOWNLOAD_COLOR)
		DrawCircle(maxY/2, maxX/2+1, radius, -90+downloadAngle, 90, CIRCLE_WIDTH, screen)
		screen.ColorOff(DOWNLOAD_COLOR)
	}

	screen.MovePrint(maxY/2+7, maxX/2-len(latencyText)/2, latencyText)
	screen.MovePrint(maxY/2-7, maxX/2-len(speedText)/2, speedText)

	uploadText = StripDigitsForDisplay(uploadText)
	downloadText = StripDigitsForDisplay(downloadText)

	PrintDigit(uploadText, UPLOAD_TEXT_COLOR, maxX/2-(len(uploadText)*6)/2, maxY/2-6, screen)
	PrintDigit(downloadText, DOWNLOAD_TEXT_COLOR, maxX/2-(len(uploadText)*6)/2, maxY/2, screen)

	screen.Refresh()

}

func DrawCircle(y int, x int, radius float64, angleFrom int, angleTo int, width float64, window *gc.Window) {

	for i := angleFrom; i < angleTo; i++ {
		for j := radius; j < radius+width; j++ {
			radAngle := float64(i) * 0.0174533
			sin, cos := math.Sincos(radAngle)
			cY := j * sin;
			cX := (j + 10) * cos;
			window.MovePrint(y+int(cY), x+int(cX), "X")
		}
	}

}

// Show the error screen
func ShowErrorScreen(screen *gc.Window, err error) {
	screen.Erase()
	screen.Refresh()
	screen.ColorOn(ERROR_COLOR)
	screen.SetBackground(gc.ColorPair(ERROR_COLOR))
	screen.Printf("Couldn't connect to the controller, double check the URL and your credentials. Retrying soon \n\n %q", err)
	screen.Refresh()
}

// Update the bar to set the color the size and the borders
func UpdateBar(screen *gc.Window, percent float64, x int, color int16) {
	maxY,_ := screen.MaxYX()

	newUploadHeight, newUploadY := CalculateNewHeightAndY(percent, maxY)

	if newUploadHeight == 0 {
		newUploadHeight = 1
	}

	if newUploadY == maxY{
		newUploadY -= 1
	}

	screen.ColorOn(color)

	//fmt.Printf("new Y: %v   %v/%v \n", newUploadY, newUploadHeight, maxY)

	for i := newUploadY; i <= newUploadY + newUploadHeight; i++ {
		for j := 0; j < BAR_WIDTH; j++{
			screen.MovePrint(i, x+j, "X")
		}
	}
	screen.ColorOff(color)

}

// calculate the new height and Y position of a bar
func CalculateNewHeightAndY(percent float64, maxY int) (int, int) {

	newHeight := int(float64(maxY) * (percent / 100))

	newY := maxY - newHeight
	return newHeight, newY
}

// Rounds a number
func Round(x, unit float64) float64 {
	return math.Round(x/unit) * unit
}

//Gets the configuration
func openConfig(configFile string) Configuration {
	var conf Configuration
	if _, err := toml.DecodeFile(configFile, &conf); err != nil {
		// handle error
		fmt.Println("SOMETHING WRONG !")
		panic(err)
	}

	return conf
}

// Converts bytes (from controler) to Mbps
func bytesToMebibit(bytes float64) float64 {
	return bytes / 131072
}

// get the actual throughput information
func getInfo(url string, site string, client *http.Client) (float64, float64, float64, error) {

	resp, err := client.Get(url + "/api/s/" + site + "/stat/health")

	if err != nil {
		return 0, 0, 0, err
	}

	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)

	var i interface{}

	if err := json.Unmarshal([]byte(body), &i); err != nil {
		return 0, 0, 0, err
	}

	json, isset := i.(map[string]interface{})["data"]

	if !isset || len(json.([]interface{})) < 3 {
		return 0, 0, 0, errors.New("couldn't read the data from the controller response, check your credentials or the site name might be wrong")
	}

	data := json.([]interface{})[2].(map[string]interface{})

	if data["latency"] != nil && data["tx_bytes-r"] != nil && data["rx_bytes-r"] != nil {
		latency := data["latency"].(float64)
		upload := data["tx_bytes-r"].(float64)
		download := data["rx_bytes-r"].(float64)

		return latency, upload, download, nil
	}else{
		return 0,0,0, errors.New("couldn't read the data from the controller response, check your credentials or the site name might be wrong")
	}

}

// login to the controller
func login(url string, username string, password string, client *http.Client) error {

	payload := strings.NewReader("{\n\t\"username\": \"" + username + "\",\n\t\"password\":\"" + password + "\"\n}")
	resp, err := client.Post(url+"/api/login", "application/json", payload)
	if err != nil {
		// handle error
		return err
	}

	defer resp.Body.Close()

	return nil
}

// digit printing functions

func StripDigitsForDisplay(digit string) string {

	digit = digit[0:5]
	if digit[len(digit)-1] == '.' {
		digit = digit[0:4]
	}

	return digit
}

func PrintDigit(digit string, color int16, x int, y int, screen *gc.Window) {
	screen.ColorOn(color)
	width := 0
	for _, char := range digit {

		switch char {
		case '0':
			width += Print0(x+width, y, screen)
			break
		case '1':
			width += Print1(x+width, y, screen)
			break
		case '2':
			width += Print2(x+width, y, screen)
			break
		case '3':
			width += Print3(x+width, y, screen)
			break
		case '4':
			width += Print4(x+width, y, screen)
			break
		case '5':
			width += Print5(x+width, y, screen)
			break
		case '6':
			width += Print6(x+width, y, screen)
			break
		case '7':
			width += Print7(x+width, y, screen)
			break
		case '8':
			width += Print8(x+width, y, screen)
			break
		case '9':
			width += Print9(x+width, y, screen)
			break
		case '.':
			width += PrintDot(x+width, y, screen)
			break
		case '^':
			width += PrintUp(x+width, y, screen)
			break
		case '-':
			width += PrintDown(x+width, y, screen)
			break

		}
	}

	screen.ColorOff(color)

}

func Print0(x int, y int, screen *gc.Window) int {

	screen.MovePrint(y+0, x, "  ___  ")
	screen.MovePrint(y+1, x, " /###\\ ")
	screen.MovePrint(y+2, x, "|#| |#|")
	screen.MovePrint(y+3, x, "|#| |#|")
	screen.MovePrint(y+4, x, "|#| |#|")
	screen.MovePrint(y+5, x, " \\###/ ")

	return 7

}

func Print1(x int, y int, screen *gc.Window) int {

	screen.MovePrint(y+0, x, " __ ")
	screen.MovePrint(y+1, x, "/##|")
	screen.MovePrint(y+2, x, " |#|")
	screen.MovePrint(y+3, x, " |#|")
	screen.MovePrint(y+4, x, " |#|")
	screen.MovePrint(y+5, x, " |#|")

	return 4

}

func Print2(x int, y int, screen *gc.Window) int {

	screen.MovePrint(y+0, x, "___   ")
	screen.MovePrint(y+1, x, "|###\\ ")
	screen.MovePrint(y+2, x, "   )#|")
	screen.MovePrint(y+3, x, "  /#/ ")
	screen.MovePrint(y+4, x, " /#/_ ")
	screen.MovePrint(y+5, x, "|####|")

	return 6
}

func Print3(x int, y int, screen *gc.Window) int {

	screen.MovePrint(y+0, x, " ____  ")
	screen.MovePrint(y+1, x, "|####\\ ")
	screen.MovePrint(y+2, x, "  __)#|")
	screen.MovePrint(y+3, x, " |###< ")
	screen.MovePrint(y+4, x, " ___)#|")
	screen.MovePrint(y+5, x, "|####/ ")

	return 7
}

func Print4(x int, y int, screen *gc.Window) int {

	screen.MovePrint(y+0, x, " _  _   ")
	screen.MovePrint(y+1, x, "|#||#|  ")
	screen.MovePrint(y+2, x, "|#||#|_ ")
	screen.MovePrint(y+3, x, "|######|")
	screen.MovePrint(y+4, x, "   |#|  ")
	screen.MovePrint(y+5, x, "   |#|  ")

	return 8
}

func Print5(x int, y int, screen *gc.Window) int {

	screen.MovePrint(y+0, x, " _____ ")
	screen.MovePrint(y+1, x, "|#####|")
	screen.MovePrint(y+2, x, "|#|__  ")
	screen.MovePrint(y+3, x, "|####\\ ")
	screen.MovePrint(y+4, x, " ___)#|")
	screen.MovePrint(y+5, x, "|####/ ")

	return 7
}
func Print6(x int, y int, screen *gc.Window) int {

	screen.MovePrint(y+0, x, "   __  ")
	screen.MovePrint(y+1, x, "  /#/  ")
	screen.MovePrint(y+2, x, " /#/_  ")
	screen.MovePrint(y+3, x, "|####\\ ")
	screen.MovePrint(y+4, x, "|#(_)#|")
	screen.MovePrint(y+5, x, " \\###/ ")

	return 7
}
func Print7(x int, y int, screen *gc.Window) int {

	screen.MovePrint(y+0, x, " ______ ")
	screen.MovePrint(y+1, x, "|######|")
	screen.MovePrint(y+2, x, "    /#/ ")
	screen.MovePrint(y+3, x, "   /#/  ")
	screen.MovePrint(y+4, x, "  /#/   ")
	screen.MovePrint(y+5, x, " /#/    ")

	return 8
}
func Print8(x int, y int, screen *gc.Window) int {

	screen.MovePrint(y+0, x, "  ___  ")
	screen.MovePrint(y+1, x, " /###\\ ")
	screen.MovePrint(y+2, x, "|#(_)#|")
	screen.MovePrint(y+3, x, " >###< ")
	screen.MovePrint(y+4, x, "|#(_)#|")
	screen.MovePrint(y+5, x, " \\###/ ")

	return 7
}
func Print9(x int, y int, screen *gc.Window) int {

	screen.MovePrint(y+0, x, "  ___  ")
	screen.MovePrint(y+1, x, " /###\\ ")
	screen.MovePrint(y+2, x, "|#(_)#|")
	screen.MovePrint(y+3, x, " \\####|")
	screen.MovePrint(y+4, x, "   /#/ ")
	screen.MovePrint(y+5, x, "  /#/  ")

	return 7
}
func PrintDot(x int, y int, screen *gc.Window) int {

	screen.MovePrint(y+0, x, "   ")
	screen.MovePrint(y+1, x, "   ")
	screen.MovePrint(y+2, x, "   ")
	screen.MovePrint(y+3, x, "   ")
	screen.MovePrint(y+4, x, " _ ")
	screen.MovePrint(y+5, x, "(#)")

	return 3
}

func PrintUp(x int, y int, screen *gc.Window) int {

	screen.MovePrint(y+0, x, "   ")
	screen.MovePrint(y+1, x, "   ")
	screen.MovePrint(y+2, x, " /#\\ ")
	screen.MovePrint(y+3, x, "/###\\")
	screen.MovePrint(y+4, x, "   ")
	screen.MovePrint(y+5, x, "   ")

	return 6
}

func PrintDown(x int, y int, screen *gc.Window) int {

	screen.MovePrint(y+0, x, "   ")
	screen.MovePrint(y+1, x, "   ")
	screen.MovePrint(y+2, x, "\\###/")
	screen.MovePrint(y+3, x, " \\#/ ")
	screen.MovePrint(y+4, x, "   ")
	screen.MovePrint(y+5, x, "   ")

	return 6
}
