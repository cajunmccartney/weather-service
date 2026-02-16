# OpenWeather API Examples

This document provides examples of OpenWeather API calls used by the weather service for both version 2.5 and 3.0.

---

## API Version Support

The weather service supports both OpenWeather API versions:

### Version 2.5 (Default - Free Tier)

**Endpoint:** `https://api.openweathermap.org/data/2.5/weather`

**Pricing:** Free (60 calls/minute, 1M calls/month)

**Requirements:** API key only (no payment method)

**Supported inputs:** Location names only

**API call:**
```bash
curl "https://api.openweathermap.org/data/2.5/weather?q=London&appid=YOUR_KEY&units=imperial"
```

**Response:**
```json
{
  "coord": {"lon": -0.1257, "lat": 51.5085},
  "weather": [{"main": "Clouds", "description": "broken clouds"}],
  "main": {"temp": 45.32, "humidity": 72},
  "wind": {"speed": 8.05},
  "name": "London"
}
```

### Version 3.0 (Optional - Requires Subscription)

**Endpoint:** `https://api.openweathermap.org/data/3.0/onecall`

**Pricing:** 1,000 calls/day free, then $0.15 per 100 calls

**Requirements:** API key + payment method on file

**Supported inputs:** Location names (geocoded) OR coordinates (direct)

**API call sequence:**
1. Geocode: `https://api.openweathermap.org/geo/1.0/direct?q=London&limit=1&appid=YOUR_KEY`
2. Weather: `https://api.openweathermap.org/data/3.0/onecall?lat=51.5074&lon=-0.1278&appid=YOUR_KEY&units=imperial&exclude=minutely,hourly,daily,alerts`

### Switching Between Versions

Set the `WEATHER_API_VERSION` environment variable:

```bash
# Use 2.5 (default - free, no setup)
export WEATHER_API_VERSION=2.5
make run

# Use 3.0 (requires payment method, supports coordinates)
export WEATHER_API_VERSION=3.0
make run
```

**Kubernetes:**
```bash
# Edit deployment to use 3.0
kubectl set env deployment/weather-service WEATHER_API_VERSION=3.0
kubectl rollout restart deployment/weather-service
```

---

## 1. API 2.5 - Direct Weather Request

The 2.5 API is simpler and requires only one API call.

### Request

```bash
curl "https://api.openweathermap.org/data/2.5/weather?q=London&appid=YOUR_API_KEY&units=imperial"
```

### Parameters
* `q` - Location query (city name, city+country, ZIP code)
* `appid` - Your API key
* `units=imperial` - Fahrenheit, mph (use `metric` for Celsius, m/s)

### Response (Abbreviated)

```json
{
  "coord": {"lon": -0.1257, "lat": 51.5085},
  "weather": [
    {
      "id": 803,
      "main": "Clouds",
      "description": "broken clouds",
      "icon": "04d"
    }
  ],
  "main": {
    "temp": 45.32,
    "feels_like": 41.18,
    "pressure": 1013,
    "humidity": 72
  },
  "wind": {"speed": 8.05},
  "name": "London"
}
```

### Key Fields Used by Weather Service
* `main.temp` - Temperature (°F with `units=imperial`)
* `main.humidity` - Humidity percentage
* `wind.speed` - Wind speed (mph with `units=imperial`)
* `weather[0].main` - Weather condition (Clear, Clouds, Rain, etc.)

---

## 2. API 3.0 - Geocoding API (Location → Coordinates)

The weather service first converts location names to coordinates using the Geocoding API.

### Request

```bash
curl "https://api.openweathermap.org/geo/1.0/direct?q=London&limit=1&appid=YOUR_API_KEY"
```

### Response

```json
[
  {
    "name": "London",
    "lat": 51.5074,
    "lon": -0.1278,
    "country": "GB",
    "state": "England"
  }
]
```

### Key Fields
* `lat` - Latitude (used for weather API)
* `lon` - Longitude (used for weather API)
* `country` - ISO 3166 country code
* `state` - State/region (when applicable)

### Error Cases
Location not found:

```bash
curl "https://api.openweathermap.org/geo/1.0/direct?q=InvalidCity123&limit=1&appid=YOUR_API_KEY"
# Returns: []
```

---

## 3. API 3.0 - One Call API (Current Weather Only)

After geocoding, the service fetches current weather using coordinates.

### Request

```bash
curl "https://api.openweathermap.org/data/3.0/onecall?lat=51.5074&lon=-0.1278&appid=YOUR_API_KEY&units=imperial&exclude=minutely,hourly,daily,alerts"
```

### Parameters
* `lat` - Latitude (from geocoding)
* `lon` - Longitude (from geocoding)
* `appid` - Your API key
* `units=imperial` - Fahrenheit, mph (use `metric` for Celsius, m/s)
* `exclude=minutely,hourly,daily,alerts` - Only get current weather (saves bandwidth)

### Response (Abbreviated)

```json
{
  "lat": 51.5074,
  "lon": -0.1278,
  "timezone": "Europe/London",
  "current": {
    "dt": 1672531200,
    "temp": 45.5,
    "feels_like": 41.2,
    "pressure": 1013,
    "humidity": 72,
    "clouds": 75,
    "wind_speed": 8.5,
    "weather": [
      {
        "id": 803,
        "main": "Clouds",
        "description": "broken clouds",
        "icon": "04d"
      }
    ]
  }
}
```

### Key Fields Used by Weather Service
From `current` object:
* `temp` - Temperature (°F with `units=imperial`)
* `humidity` - Humidity percentage
* `wind_speed` - Wind speed (mph with `units=imperial`)
* `weather[0].main` - Weather condition (Clear, Clouds, Rain, etc.)
* `weather[0].description` - Detailed description

---

## 4. Complete Example: London Weather (API 3.0)

### Step 1: Geocode "London"

```bash
curl "https://api.openweathermap.org/geo/1.0/direct?q=London&limit=1&appid=YOUR_API_KEY"
```

Result: `lat=51.5074, lon=-0.1278`

### Step 2: Fetch Weather

```bash
curl "https://api.openweathermap.org/data/3.0/onecall?lat=51.5074&lon=-0.1278&appid=YOUR_API_KEY&units=imperial&exclude=minutely,hourly,daily,alerts"
```

### Formatted Response (Weather Service Format):

```json
{
  "location": "London",
  "temperature": 45.5,
  "conditions": "Clouds",
  "humidity": 72,
  "wind_speed": 8.5
}
```

---

## 5. Using Coordinates Directly (API 3.0 Only)

The weather service also accepts coordinates in "lat,lon" format, which skips the geocoding step.

### Example: New York City

**Using location name:**
```bash
curl http://localhost:8080/weather/New%20York
# Steps: Geocode "New York" → Get weather for coordinates
# API calls: 2 (geocoding + weather)
```

**Using coordinates:**
```bash
curl http://localhost:8080/weather/40.7128,-74.0060
# Steps: Use coordinates directly → Get weather
# API calls: 1 (weather only)
```

**Same response:**
```json
{
  "location": "40.7128,-74.0060",
  "temperature": 45.2,
  "conditions": "Clear",
  "humidity": 65,
  "wind_speed": 7.8
}
```

### Coordinate Format Requirements

- **Format:** `lat,lon` (comma-separated)
- **Latitude range:** -90 to 90
- **Longitude range:** -180 to 180
- **Decimals:** Any precision accepted
- **Spaces:** Optional around comma (both `51.5074,-0.1278` and `51.5074, -0.1278` work)

### Examples

```bash
# London
curl http://localhost:8080/weather/51.5074,-0.1278

# Tokyo
curl http://localhost:8080/weather/35.6762,139.6503

# Sydney
curl http://localhost:8080/weather/-33.8688,151.2093

# São Paulo (southern hemisphere)
curl http://localhost:8080/weather/-23.5505,-46.6333
```

### When to Use Coordinates vs Location Names

**Use coordinates when:**
- You already have lat/lon data
- You want faster responses (skips geocoding)
- You want precise control over location
- You're querying the same location repeatedly (cache still works)

**Use location names when:**
- More user-friendly
- Don't have coordinates
- Want human-readable cache keys
- Querying by city/region name

---

## 6. Other Supported Data Types (API 3.0, Not Used by Service)

The One Call API 3.0 supports additional data via the `exclude` parameter:

### Minutely Forecast (Next Hour)

```bash
# Remove "minutely" from exclude parameter
curl "https://api.openweathermap.org/data/3.0/onecall?lat=51.5074&lon=-0.1278&appid=YOUR_API_KEY&units=imperial&exclude=hourly,daily,alerts"
```

Returns minute-by-minute precipitation for next 60 minutes.

### Hourly Forecast (48 Hours)

```bash
# Remove "hourly" from exclude parameter
curl "https://api.openweathermap.org/data/3.0/onecall?lat=51.5074&lon=-0.1278&appid=YOUR_API_KEY&units=imperial&exclude=minutely,daily,alerts"
```

Returns hourly forecast for next 48 hours.

### Daily Forecast (8 Days)

```bash
# Remove "daily" from exclude parameter
curl "https://api.openweathermap.org/data/3.0/onecall?lat=51.5074&lon=-0.1278&appid=YOUR_API_KEY&units=imperial&exclude=minutely,hourly,alerts"
```

Returns daily forecast for next 8 days.

### Weather Alerts

```bash
# Remove "alerts" from exclude parameter
curl "https://api.openweathermap.org/data/3.0/onecall?lat=51.5074&lon=-0.1278&appid=YOUR_API_KEY&units=imperial&exclude=minutely,hourly,daily"
```

Returns government weather alerts for the location.

---

## 7. Error Handling Examples

### Invalid API Key

```bash
curl "https://api.openweathermap.org/data/3.0/onecall?lat=51.5074&lon=-0.1278&appid=invalid&units=imperial"
```

Response:

```json
{
  "cod": 401,
  "message": "Invalid API key. Please see http://openweathermap.org/faq#error401 for more info."
}
```

### Rate Limit Exceeded

```bash
# After exceeding 1,000 calls/day on free tier
```

Response:

```json
{
  "cod": 429,
  "message": "Your account is temporary blocked due to exceeding of requests limitation of your subscription type."
}
```

Weather Service Behavior: Returns 429 → Triggers retry → Eventually serves stale cache if available

### Invalid Coordinates

```bash
curl "https://api.openweathermap.org/data/3.0/onecall?lat=999&lon=999&appid=YOUR_API_KEY&units=imperial"
```

Response:

```json
{
  "cod": "400",
  "message": "wrong latitude"
}
```

---

## 8. Testing the Weather Service

### Test with curl

```bash
# Start service with API 2.5 (default)
export WEATHER_API_KEY=your_key_here
export WEATHER_API_VERSION=2.5
make run

# Test with location names (2.5)
curl http://localhost:8080/weather/London
curl http://localhost:8080/weather/Tokyo
curl http://localhost:8080/weather/New%20York

# Start service with API 3.0
export WEATHER_API_VERSION=3.0
make run

# Test with location names (3.0 - uses geocoding)
curl http://localhost:8080/weather/London

# Test with coordinates (3.0 only - skips geocoding)
curl http://localhost:8080/weather/51.5074,-0.1278
curl http://localhost:8080/weather/35.6762,139.6503
curl http://localhost:8080/weather/40.7128,-74.0060

# Test invalid location (should return error)
curl http://localhost:8080/weather/InvalidCity12345

# Test invalid coordinates (3.0 only, should return error)
curl http://localhost:8080/weather/999,999
```

### Monitor API Usage

```bash
# Check cache effectiveness (reduces API calls)
curl http://localhost:8080/metrics | grep cache_hits_total

# Check upstream API calls
curl http://localhost:8080/metrics | grep external_api_requests_total

# Check for errors
curl http://localhost:8080/metrics | grep external_api_failures_total
```

---

## 9. Cost Estimation

### API 2.5 (Free Forever)
**Any volume:** $0.00/day (60 calls/min, 1M calls/month)

### API 3.0 Cost Scenarios

**Scenario: 1,000 Unique Locations per Day**

**Without Cache:**
* Geocoding: 1,000 calls (free)
* Weather: 1,000 calls ($0.00 within free tier)
* Total: $0.00/day

**With 5-Minute Cache (80% hit rate):**
* Geocoding: 200 calls (free)
* Weather: 200 calls ($0.00 within free tier)
* Total: $0.00/day

**Scenario: 10,000 Requests per Day (50 popular locations)**

**With Cache (95% hit rate for popular locations):**
* Geocoding: 500 calls (free)
* Weather: 500 calls ($0.00 within free tier)
* Total: $0.00/day

**Scenario: Exceeding Free Tier**

**2,000 Weather API Calls per Day (3.0):**
* Free tier: 1,000 calls
* Overage: 1,000 calls = 10 × 100-call blocks
* Cost: 10 × $0.15 = $1.50/day = $45/month

**Cost Optimization:**
* Use API 2.5 for unlimited free calls
* Cache reduces calls by 80-95%
* Rate limiting prevents abuse
* Stale-on-error reduces calls during outages

---

## References

* Weather API 2.5: https://openweathermap.org/current
* One Call API 3.0: https://openweathermap.org/api/one-call-3
* Geocoding API: https://openweathermap.org/api/geocoding-api
* Pricing: https://openweathermap.org/price
* API Response Examples: https://openweathermap.org/api/one-call-3#example
