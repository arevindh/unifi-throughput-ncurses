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
	Username string
	Password string
	Site     string
	Url      string
}

const CYAN = 1
const BLUE = 2
const CYAN_BLACK = 3
const BLUE_BLACK = 4
const BLACK = 5
const WHITE_RED = 6
const DEFAULT_CONFIG_FOLDER = "/.config/unifi-throughput"
const DEFAULT_CONFIG_PATH = DEFAULT_CONFIG_FOLDER + "/config.toml"
const VERSION = "1.0"

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

	config := []byte("#Controller URL, do not add tailing /\nurl=\"https://demo.ubnt.com\"\n# Name of the site\nsite = \"default\"\n#credentials to login to the controller\nusername = \"superadmin\"\npassword =\"\"")
	err := ioutil.WriteFile(GetDefaultConfigPath(), config, 0644)
	if err != nil {
		fmt.Println(err)
	} else {
		fmt.Println("Config created at", GetDefaultConfigPath())
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

	//upload.Color(gc.C_BLUE)
	if err := gc.InitPair(CYAN, gc.C_CYAN, gc.C_CYAN); err != nil {
		panic(err)
	}

	if err := gc.InitPair(BLUE, gc.C_MAGENTA, gc.C_MAGENTA); err != nil {
		panic(err)
	}

	if err := gc.InitPair(BLUE_BLACK, gc.C_MAGENTA, gc.C_BLACK); err != nil {
		panic(err)
	}

	if err := gc.InitPair(CYAN_BLACK, gc.C_CYAN, gc.C_BLACK); err != nil {
		panic(err)
	}
	if err := gc.InitPair(BLACK, gc.C_BLACK, gc.C_BLACK); err != nil {
		panic(err)
	}

	if err := gc.InitPair(WHITE_RED, gc.C_WHITE, gc.C_RED); err != nil {
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

	stdscr.Overlay(upload)
	stdscr.Overlay(download)

	stdscr.Refresh()

	go GetData(config, client, stdscr, upload, download)

	//loop := true
	//for loop  {
	//	switch char := stdscr.GetChar(); char{
	//	default:
	//		fmt.Println(char)
	//	}
	//}
	stdscr.GetChar()

}

// Starts the actual logic of the application
func GetData(config Configuration, client *http.Client, screen *gc.Window, uploadBar *gc.Window, downloadBar *gc.Window) {
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

			DisplayData(latency, upload, download, maxValue, screen, uploadBar, downloadBar);
		}
		time.Sleep(3 * time.Second)
	}
}

// Display stuff on ncurses
func DisplayData(latency float64, upload float64, download float64, maxValue float64, screen *gc.Window, uploadBar *gc.Window, downloadBar *gc.Window) {
	//keeping the max value

	//getting the speed in mbps
	readableUpload := bytesToMebibit(upload)
	readableDownload := bytesToMebibit(download)

	maxUploadPercent := (upload / maxValue) * 100
	maxDownloadPercent := (download / maxValue) * 100

	maxY, maxX := screen.MaxYX()

	//uploadText := "Ul: " + strconv.FormatFloat(readableUpload, 'f', 2, 64) + "mbps"
	//downloadText := "Dl: " + strconv.FormatFloat(readableDownload, 'f', 2, 64) + "mbps";
	latencyText := "Latency: " + strconv.FormatFloat(latency, 'f', 0, 64) + "ms"
	speedText := "Speeds (mbps)"


	// If we have more than 4 digits, we convert to gbps, we have time to see up to tbps //todo: remind me in 50 years
	if readableDownload >= 1000 || readableUpload >= 1000{
		readableDownload /= 1000
		readableUpload /= 1000
		speedText = "Speeds (gbps)"
	}

	uploadText := "^" + strconv.FormatFloat(readableUpload, 'f', 2, 64)
	downloadText := "-" + strconv.FormatFloat(readableDownload, 'f', 2, 64)

	screen.Erase()
	screen.Refresh()

	UpdateBar(uploadBar, maxUploadPercent, maxY, BLUE)

	UpdateBar(downloadBar, maxDownloadPercent, maxY, CYAN)

	//textXOffset := -8
	//screen.ColorOn(BLUE_BLACK)
	//screen.MovePrint(maxY/2-1, maxX/2+textXOffset, uploadText)
	//screen.ColorOn(CYAN_BLACK)
	//screen.MovePrint(maxY/2, maxX/2+textXOffset, downloadText)
	//screen.ColorOff(CYAN_BLACK)
	screen.MovePrint(maxY/2+7, maxX/2-len(latencyText)/2, latencyText)
	screen.MovePrint(maxY/2-7, maxX/2-len(speedText)/2, speedText)


	uploadText = StripDigitsForDisplay(uploadText)
	downloadText = StripDigitsForDisplay(downloadText)

	PrintDigit(uploadText, BLUE_BLACK, maxX/2-(len(uploadText)*6)/2, maxY/2-6, screen)
	PrintDigit(downloadText, CYAN_BLACK, maxX/2-(len(uploadText)*6)/2, maxY/2, screen)

	screen.Refresh()

}

// Show the error screen
func ShowErrorScreen(screen *gc.Window, err error) {
	screen.Erase()
	screen.Refresh()
	screen.ColorOn(WHITE_RED)
	screen.SetBackground(gc.ColorPair(WHITE_RED))
	screen.Printf("Couldn't connect to the controller, double check the URL and your credentials. Retrying soon \n\n %q", err)
	screen.Refresh()
}

// Update the bar to set the color the size and the borders
func UpdateBar(bar *gc.Window, percent float64, maxY int, color int16) {

	newUploadHeight, newUploadY := CalculateNewHeightAndY(percent, maxY)
	_, uploadWidth := bar.MaxYX()
	_, uploadX := bar.YX()
	bar.Resize(newUploadHeight, uploadWidth)

	bar.ColorOn(color)
	bar.MoveWindow(newUploadY, uploadX)
	bar.Border(gc.ACS_VLINE, gc.ACS_VLINE, gc.ACS_HLINE, gc.ACS_HLINE,
		gc.ACS_ULCORNER, gc.ACS_URCORNER, gc.ACS_LLCORNER, gc.ACS_LRCORNER)
	bar.Color(color)
	bar.ColorOff(color)
	bar.SetBackground(gc.ColorPair(color))
	bar.Refresh()

}

// calculate the new height and Y position of a bar
func CalculateNewHeightAndY(percent float64, maxY int) (int, int) {

	newHeight := int(float64(maxY) * (percent / 100))
	//fmt.Println("new height", newHeight)
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
	//fmt.Printf("response %q\n", body)

	if err := json.Unmarshal([]byte(body), &i); err != nil {
		panic(err)
	}

	json, isset := i.(map[string]interface{})["data"]

	if !isset || len(json.([]interface{})) < 3 {
		return 0, 0, 0, errors.New("couldn't read the data from the controller response, check your credentials or the site name might be wrong")
	}

	data := json.([]interface{})[2].(map[string]interface{})

	latency := data["latency"].(float64)
	upload := data["tx_bytes-r"].(float64)
	download := data["rx_bytes-r"].(float64)
	//return strconv.Atoi(www.([]interface{})["latency"]), www["tx_bytes-r"], www["rx_bytes-r"]
	return latency, upload, download, nil
	//return 0, 0, 0
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
	//body, err := ioutil.ReadAll(resp.Body)
	//fmt.Printf("response %q\n", body)
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
	screen.MovePrint(y+3, x, "/#/#\\")
	screen.MovePrint(y+4, x, "   ")
	screen.MovePrint(y+5, x, "   ")

	return 6
}

func PrintDown(x int, y int, screen *gc.Window) int {

	screen.MovePrint(y+0, x, "   ")
	screen.MovePrint(y+1, x, "   ")
	screen.MovePrint(y+2, x, "\\#\\#/")
	screen.MovePrint(y+3, x, " \\#/ ")
	screen.MovePrint(y+4, x, "   ")
	screen.MovePrint(y+5, x, "   ")

	return 6
}
