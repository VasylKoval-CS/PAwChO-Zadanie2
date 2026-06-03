	package main

	import (
		"encoding/json"
		"fmt"
		"html/template"
		"log"
		"net/http"
		"os"
		"time"
	)

	// Mapa współrzędnych dla Open-Meteo API
	var cities = map[string]struct{ Lat, Lon string }{
		"Warszawa": {"52.2297", "21.0122"},
		"Lublin":   {"51.2500", "22.5667"},
		"Kyiv":     {"50.4501", "30.5234"},
	}

	// Struktura odpowiedzi JSON z API
	type WeatherResponse struct {
		CurrentWeather struct {
			Temperature float64 `json:"temperature"`
			Windspeed   float64 `json:"windspeed"`
		} `json:"current_weather"`
	}

	func main() {
		// 1. HEALTHCHECK
		// Uruchomienie z flagą '-health' testuje lokalny endpoint /health
		if len(os.Args) > 1 && os.Args[1] == "-health" {
			resp, err := http.Get("http://127.0.0.1:8080/health")
			if err != nil || resp.StatusCode != 200 {
				os.Exit(1) // Status: unhealthy
			}
			os.Exit(0) // Status: healthy
		}

		// 2. SERVER MODE
		port := "8080"
		author := "Vasyl Koval"

		// Logowanie przy starcie
		log.Printf("=== Aplikacja uruchomiona ===")
		log.Printf("Data uruchomienia: %s", time.Now().Format(time.RFC1123))
		log.Printf("Autor: %s", author)
		log.Printf("Nasłuchiwanie na porcie TCP: %s", port)

		// Główny handler aplikacji
		http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			city := r.URL.Query().Get("city")
			var weatherData string

			// Pobieranie danych pogodowych, jeśli wybrano miasto
			if coords, ok := cities[city]; ok {
				url := fmt.Sprintf("https://api.open-meteo.com/v1/forecast?latitude=%s&longitude=%s&current_weather=true", coords.Lat, coords.Lon)
				resp, err := http.Get(url)
				if err == nil {
					defer resp.Body.Close()
					var wr WeatherResponse
					if json.NewDecoder(resp.Body).Decode(&wr) == nil {
						weatherData = fmt.Sprintf("Temperatura: %.1f°C, Wiatr: %.1f km/h", wr.CurrentWeather.Temperature, wr.CurrentWeather.Windspeed)
					}
				}
			}

			// Prosty interfejs UI
			html := `
			<!DOCTYPE html>
			<html>
			<head><title>Pogoda - {{.Author}}</title><meta charset="utf-8"></head>
			<body style="font-family: sans-serif; padding: 20px;">
				<h1>Wybierz lokalizację</h1>
				<form method="GET">
					<select name="city">
						<option value="Warszawa">Warszawa (Polska)</option>
						<option value="Lublin">Lublin (Polska)</option>
						<option value="Kyiv">Kyiv (Ukraina)</option>
					</select>
					<button type="submit">Sprawdź pogodę</button>
				</form>
				{{if .City}}
					<div style="margin-top: 20px; padding: 10px; background: #f0f0f0; border-radius: 5px; display: inline-block;">
						<h2>{{.City}}</h2>
						<p><strong>{{.Weather}}</strong></p>
					</div>
				{{end}}
			</body>
			</html>
			`
			tmpl, _ := template.New("webpage").Parse(html)
			tmpl.Execute(w, struct{ City, Weather, Author string }{city, weatherData, author})
		})

		// Endpoint używany przez mechanizm healthcheck
		http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("OK"))
		})

		log.Fatal(http.ListenAndServe(":"+port, nil))
	}
