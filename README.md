# PAwChO - Zadanie 2: CI/CD Pipeline

## 1. Realizacja wymagań
* **Dwie architektury (linux/arm64, linux/amd64):** Zastosowano emulator **QEMU** oraz **Docker Buildx**.
* **Cache na DockerHub:** Wdrożono zdalną pamięć podręczną (`registry cache`). Uzasadnienie: Flaga `mode=max` gwarantuje, że do repozytorium na DockerHub eksportowane są warstwy ze wszystkich etapów wieloetapowego `Dockerfile`. Znacząco redukuje to czas kolejnych uruchomień pipeline'u.
* **Skanowanie CVE i bramkowanie:** Do testu bezpieczeństwa użyto skanera **Trivy**. Proces podzielono: budowany jest obraz lokalny (`load: true`), a następnie Trivy go skanuje. Użycie parametru `exit-code: '1'` i `severity: 'CRITICAL,HIGH'` sprawia, że w przypadku znalezienia groźnych podatności pipeline zostaje przerwany. Docelowy `push` do GHCR wykonuje się tylko po pomyślnym zaliczeniu testu.

---

## 2. Tagowanie Obrazów

Zarządzanie metadanymi i tagami zrealizowano za pomocą oficjalnej akcji `docker/metadata-action@v5`.

1. **Tag `latest`** (`type=raw,value=latest`): Generowany automatycznie tylko dla głównej gałęzi (`main`). Ułatwia to użytkownikom pobranie najnowszej stabilnej wersji.
2. **Tag Immutable** (`type=sha,format=long`): Generowanie unikalnego tagu na podstawie hasha. Uzasadnienie: Tagi z hashem gwarantują pełną identyfikowalność (traceability), co jest kluczową praktyką pozwalającą bezbłędnie powiązać gotowy obraz z konkretną wersją kodu źródłowego.

---

## 3. Wyniki działania

Poniżej przedstawiono dowody poprawnego działania łańcucha CI/CD:

### Sukces wykonania łańcucha GitHub Actions
<img width="2866" height="1021" alt="Actions" src="https://github.com/user-attachments/assets/7a23eac8-b77d-4574-98d7-56e5242de83d" />

### Poprawne przejście skanowania Trivy (0 podatności)
<img width="2257" height="1556" alt="Trivy_build_scan_and_push" src="https://github.com/user-attachments/assets/e4236b62-ea2f-40df-8bdb-2c4826517577" />
*Trivy nie znalazł żadnych podatności CRITICAL ani HIGH, co pozwoliło na wykonanie kolejnego kroku (Push do GHCR).*

### Opublikowany pakiet w GitHub Container Registry (GHCR)
<img width="1413" height="226" alt="package" src="https://github.com/user-attachments/assets/6c14339d-3fee-4f6f-8bb3-44a7ac6bcdc0" />

---

## 4. Kody źródłowe i konfiguracja

### `.github/workflows/pipeline.yml`
```yaml
name: CI/CD Pipeline Zadanie 2

# pipeline uruchamia się przy każdym pushu na gałąź main
on:
  push:
    branches:
      - main

# Definicja zmiennych środowiskowych
env:
  REGISTRY: ghcr.io
  IMAGE_NAME: ${{ github.repository }}

jobs:
  build-scan-and-push:
    runs-on: ubuntu-latest
    # Nadanie uprawnień do zapisu w GitHub Container Registry (ghcr.io)
    permissions:
      contents: read
      packages: write

    steps:
      # 1. Pobranie kodu źródłowego z repozytorium
      - name: Checkout repository
        uses: actions/checkout@v4

      # 2. Konfiguracja QEMU (potrzebne do budowy obrazów dla architektury ARM)
      - name: Set up QEMU
        uses: docker/setup-qemu-action@v3

      # 3. Konfiguracja Docker Buildx (rozszerzony builder wspierający multi-arch i zaawansowany cache)
      - name: Set up Docker Buildx
        uses: docker/setup-buildx-action@v3

      # 4. Logowanie do DockerHub (w celu obsługi zewnętrznego cache)
      - name: Log in to DockerHub
        uses: docker/login-action@v3
        with:
          username: ${{ secrets.DOCKERHUB_USERNAME }}
          password: ${{ secrets.DOCKERHUB_TOKEN }}

      # 5. Logowanie do GitHub Container Registry (docelowe miejsce dla obrazu)
      - name: Log in to the Container registry
        uses: docker/login-action@v3
        with:
          registry: ${{ env.REGISTRY }}
          username: ${{ github.actor }}
          password: ${{ secrets.GITHUB_TOKEN }}

      # 6. Strategia tagowania
      - name: Extract metadata (tags, labels) for Docker
        id: meta
        uses: docker/metadata-action@v5
        with:
          images: ${{ env.REGISTRY }}/${{ env.IMAGE_NAME }}
          tags: |
            # Tagowanie najnowszej wersji
            type=raw,value=latest,enable={{is_default_branch}}
            # Tagowanie unikalnym hashem commita (immutable tags)
            type=sha,format=long

      # 7. Budowa lokalnego obrazu (tylko amd64) na potrzeby skanowania
      # (Docker nie pozwala na łatwy eksport multi-arch do lokalnego demona)
      - name: Build local image for scanning
        uses: docker/build-push-action@v5
        with:
          context: .
          load: true
          platforms: linux/amd64
          tags: localbuild:test
          # Wykorzystujemy cache z DockerHub do przyspieszenia tej budowy
          cache-from: type=registry,ref=${{ secrets.DOCKERHUB_USERNAME }}/pawcho-cache:main

      # 8. Skanowanie podatności narzędziem Trivy
      # Pipeline zostanie przerwany jeśli wykryje podatności CRITICAL lub HIGH
      - name: Run Trivy vulnerability scanner
        uses: aquasecurity/trivy-action@master
        with:
          image-ref: 'localbuild:test'
          format: 'table'
          exit-code: '1'
          ignore-unfixed: true
          vuln-type: 'os,library'
          severity: 'CRITICAL,HIGH'

      # 9. Budowa Multi-Arch i Push do GHCR
      - name: Build and push multi-arch image
        uses: docker/build-push-action@v5
        with:
          context: .
          push: true
          # Wsparcie dla dwóch architektur
          platforms: linux/amd64,linux/arm64
          tags: ${{ steps.meta.outputs.tags }}
          labels: ${{ steps.meta.outputs.labels }}
          # Konfiguracja zewnętrznego cache na DockerHub
          cache-from: type=registry,ref=${{ secrets.DOCKERHUB_USERNAME }}/pawcho-cache:main
          cache-to: type=registry,ref=${{ secrets.DOCKERHUB_USERNAME }}/pawcho-cache:main,mode=max
```

### `Dockerfile`
```dockerfile
# ==========================================
# ETAP 1: Budowanie aplikacji (Builder)
# ==========================================

# Updated from golang:1.22-alpine
FROM golang:1.25-alpine AS builder

# Ustawienie katalogu roboczego
WORKDIR /app

# Instalacja certyfikatów CA (wymagane do zapytań HTTPS z obrazu scratch)
RUN apk --no-cache add ca-certificates

# Kopiowanie kodu źródłowego
COPY main.go .

# Kompilacja statyczna:
# -ldflags="-w -s" - usuwa informacje debugowania, zmniejszając rozmiar pliku binarnego
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -a -installsuffix cgo -o webapp main.go

# ==========================================
# ETAP 2: Obraz docelowy (Release)
# ==========================================
# Użycie pustego obrazu
FROM scratch

# Metadane OCI
LABEL org.opencontainers.image.authors="Vasyl Koval"
LABEL org.opencontainers.image.title="Zadanie 1 - Pogoda"
LABEL org.opencontainers.image.description="Aplikacja pogodowa w Go na obrazie scratch"

# Kopiowanie certyfikatów SSL z etapu buildera
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/

# Kopiowanie skompilowanego pliku binarnego
COPY --from=builder /app/webapp /webapp

# Informacja o porcie TCP
EXPOSE 8080

# HEALTHCHECK z użyciem flagi '-health' zdefiniowanej w kodzie
# Nie używamy curl, ponieważ obraz scratch nie posiada powłoki shell
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
  CMD ["/webapp", "-health"]

# Uruchomienie aplikacji
CMD ["/webapp"]
```

### `main.go`
```go
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
```
