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
