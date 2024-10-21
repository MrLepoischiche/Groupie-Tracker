package main

import (
	"common"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"
)

const (
	ApiURL = "https://groupietrackers.herokuapp.com/api"
)

var (
	ArtistsArray   []Artist
	LocationsArray []string
	DatesArray     []string

	InfoMap      map[uint]ArtistInfo
	AllLocations map[string]LocationMap
)

type ErrorData struct {
	Code                 int
	Message, Description string
}

type Artist struct {
	Id              uint     `json:"id"`
	Image           string   `json:"image"`
	Name            string   `json:"name"`
	Members         []string `json:"members"`
	FirstAlbum      string   `json:"firstAlbum"`
	CreationDate    uint     `json:"creationDate"`
	LocationsURL    string   `json:"locations"`
	ConcertDatesURL string   `json:"concertDates"`
	RelationsURL    string   `json:"relations"`
	Locations       Locations
	ConcertDates    Dates
	Relations       Relation
}
type Locations struct {
	Id             uint     `json:"id"`
	LocationValues []string `json:"locations"`
	Dates          string   `json:"dates"`
}
type Dates struct {
	Id          uint     `json:"id"`
	DatesValues []string `json:"dates"`
}
type Relation struct {
	Id             uint                `json:"id"`
	DatesLocations map[string][]string `json:"datesLocations"`
}
type ArtistInfo struct {
	TheArtist     Artist
	TheirConcerts []string
	TheirCoords   []LocationMap
}

type LocationMap struct {
	Name      string `json:"display_name"`
	Latitude  string `json:"lat"`
	Longitude string `json:"lon"`
}

func main() {
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		sig := <-sigs

		fmt.Print("\nProgram ")
		switch sig {
		case syscall.SIGINT:
			fmt.Print("interrupted. ")
		case syscall.SIGTERM:
			fmt.Print("terminated. ")
		}
		fmt.Println("Exiting.")
		os.Exit(0)
	}()

	port := "8080"
	server := &http.Server{
		Addr:              ":" + port,
		Handler:           nil,
		ReadHeaderTimeout: 10 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       120 * time.Second,
		MaxHeaderBytes:    1 << 20,
	}

	http.HandleFunc("/", Home)
	http.HandleFunc("/detail/", Detail)
	http.HandleFunc("/search", Search)
	http.HandleFunc("/filter", Filter)

	http.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir("static"))))

	if errJson := fetchJSONData(ApiURL+"/artists", &ArtistsArray); errJson != nil {
		fmt.Fprintln(os.Stderr, errJson.Error())
		return
	}

	AllLocations = make(map[string]LocationMap, 0)
	InfoMap = make(map[uint]ArtistInfo, len(ArtistsArray))

	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		for i := 0; i < len(ArtistsArray); i++ {
			var locations Locations
			if err := fetchJSONData(ArtistsArray[i].LocationsURL, &locations); err != nil {
				fmt.Fprintln(os.Stderr, err.Error())
				break
			}

			ArtistsArray[i].Locations = locations

			for _, loc := range ArtistsArray[i].Locations.LocationValues {
				formattedLoc := FormatLocation(loc)

				if common.IndexOf(LocationsArray, formattedLoc) == -1 {
					LocationsArray = append(LocationsArray, formattedLoc)
				}
			}

			common.SelectSortArray(LocationsArray)
		}
	}()

	go func() {
		defer wg.Done()
		for i := 0; i < len(ArtistsArray); i++ {
			var dates Dates
			if err := fetchJSONData(ArtistsArray[i].ConcertDatesURL, &dates); err != nil {
				fmt.Fprintln(os.Stderr, err.Error())
				break
			}

			ArtistsArray[i].ConcertDates = dates

			for _, date := range ArtistsArray[i].ConcertDates.DatesValues {
				var formattedDate string
				formattedDate = strings.Replace(date, "*", "", 1)
				formattedDate = strings.Replace(formattedDate, "-", "/", 2)
				if common.IndexOf(DatesArray, common.StrWordRev(formattedDate)) == -1 {
					DatesArray = append(DatesArray, common.StrWordRev(formattedDate))
				}
			}

			common.SelectSortArray(DatesArray)
		}
	}()

	wg.Wait()

	fmt.Println("Server starting on http://localhost:" + port)

	go func() {
		exists := true

		coordFile, err := os.Open("coordinates.json")
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				coordFile, _ = os.Create("coordinates.json")
				exists = false
			} else {
				fmt.Fprintf(os.Stderr, "Error opening coordinates.json: %v\n", err)
			}
		}

		if !exists {
			output := "[\n"

			for _, location := range LocationsArray {
				var coords []LocationMap

				response, err := http.Get("https://nominatim.openstreetmap.org/search?q=" + FormattedToURIComponent(location) + "&format=json&limit=1&accept-language=en")
				if err != nil {
					fmt.Fprintf(os.Stderr, "Error resolving URL for %s:\n%v", location, err)
				}
				defer response.Body.Close()

				err = json.NewDecoder(response.Body).Decode(&coords)
				if err != nil {
					fmt.Println("Error decoding:", err)
					os.Exit(1)
				}

				if len(coords) == 0 {
					fmt.Fprintf(os.Stderr, "Nothing was found for query %s\n", FormattedToURIComponent(location))
					os.Exit(1)
				}

				fmt.Println(coords[0])

				line, errM := json.Marshal(coords[0])
				if errM != nil {
					fmt.Fprintln(os.Stderr, "Error marshaling:", errM.Error())
				}

				output += "\t" + string(line) + ",\n"

				AllLocations[location] = coords[0]
			}

			output = output[:len(output)-2] + "\n]"

			coordFile.Write([]byte(output))

			return
		}

		var coordsRead []LocationMap

		input, errR := os.ReadFile("coordinates.json")

		if errR != nil {
			fmt.Fprintln(os.Stderr, "Error reading file coordinates.json:", errR.Error())
			return
		}

		if len(input) == 0 {
			fmt.Fprintln(os.Stderr, "Error: Nothing was read...")
			return
		}

		if err := json.Unmarshal(input, &coordsRead); err != nil {
			fmt.Fprintln(os.Stderr, "Error unmarshaling info:", err.Error())
			return
		}

		for i := range LocationsArray {
			AllLocations[LocationsArray[i]] = coordsRead[i]
		}
	}()

	if errSrv := server.ListenAndServe(); errSrv != nil {
		log.Fatal(errSrv)
	}
}

func FormatLocation(location string) string {
	formattedLoc := location

	formattedLoc = strings.Replace(formattedLoc, "-", ", ", -1)
	formattedLoc = strings.Replace(formattedLoc, "_", " ", -1)
	formattedLoc = common.Capitalize(formattedLoc)
	formattedLoc = strings.Replace(formattedLoc, "Usa", "USA", 1)
	formattedLoc = strings.Replace(formattedLoc, "Uk", "UK", 1)
	formattedLoc = strings.Replace(formattedLoc, "Netherlands Antilles", "Curacao", 1)

	return formattedLoc
}

func FormattedToURIComponent(s string) string {
	formatted := s

	formatted = strings.Replace(strings.Replace(formatted, ", ", ",", 1), ",", "%2C", 1)
	formatted = strings.ReplaceAll(formatted, " ", "%20")

	return formatted
}
