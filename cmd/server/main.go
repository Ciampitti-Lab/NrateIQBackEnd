package main

import (
	"fmt"
	"log"
	"os"
	"strconv"
	"time"

	"github.com/JorgeJola/indnratebackend/internal/database"
	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin" // Web Framework
)

func main() {
	log.SetOutput(os.Stderr)
	fmt.Fprintln(os.Stderr, "indnrate: starting")

	if err := database.Connect(); err != nil {
		fmt.Fprintf(os.Stderr, "indnrate: database error: %v\n", err)
		log.Fatal(err)
	}

	router := gin.Default()

	router.Use(cors.New(cors.Config{
		AllowOrigins: []string{
			"http://localhost:3000", // local Next.js
		},
		AllowMethods: []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowHeaders: []string{"Origin", "Content-Type", "Authorization"},
		ExposeHeaders: []string{"Content-Length"},
		AllowCredentials: true,
		MaxAge: 12 * time.Hour,
	}))

	router.GET("/", func(c *gin.Context) {
		c.JSON(200, gin.H{
			"status": "healthy",
		})
	})


///////////////////////////////////////////////////////////////////////////////////////////////////////////////////
// EONR COUNTING
	router.GET("/onfarmtrials/eonr_count", func(c *gin.Context) {

    regionID := c.Query("region")

    if regionID == "" {
        c.JSON(400, gin.H{"error": "region parameter is required"})
        return
    }

	// Optional parameters (Nitrogen and grain price)
	nitroPriceStr := c.Query("nitro_price")
	var err error
	var nitroPrice float64

	if nitroPriceStr != "" {
		nitroPrice,err = strconv.ParseFloat(nitroPriceStr,64)
		if err != nil{
			c.JSON(400, gin.H{"error" : "Nitrogen price should be a number (Float)"})
			return
		}
	} else {
		nitroPrice = 0.4
	}

	grainPriceStr := c.Query("grain_price")

	var grainPrice float64

	if grainPriceStr != "" {
		grainPrice,err = strconv.ParseFloat(grainPriceStr,64)
		if err !=nil{
			c.JSON(400, gin.H{"error":"Grain price should be a number (Float)"})
			return
		}
	} else{
		grainPrice = 4
	}

	// Quering database
    onfarm, err := database.QueryEonrCount(regionID, nitroPrice, grainPrice)
    if err != nil {
		fmt.Printf("Database error: %v\n", err)
        c.JSON(500, gin.H{"error": "database query failed"})
        return
    }

    c.JSON(200, onfarm)


})

///////////////////////////////////////////////////////////////////////////////////////////////////////////////////
// Historical Nitrogen Prices
	router.GET("/nitro_prices", func(c *gin.Context) {

		dateStr := c.Query("date")
		source := c.Query("source")

		if dateStr == "" || source == "" {
			c.JSON(400, gin.H{"error": "date and source are required"})
			return
		}

		date, err := time.Parse("2006-01-02", dateStr)
		if err != nil {
			c.JSON(400, gin.H{
				"error": "invalid date format, use YYYY-MM-DD",
			})
			return
		}
		
		startDate := time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, time.UTC)
		endDate := startDate.Add(24 * time.Hour)
		data, err := database.QueryNitroPrices(startDate, endDate, source)
		if err != nil {
			c.JSON(500, gin.H{"error": "database query failed"})
			return
		}

		c.JSON(200, data)
	})

///////////////////////////////////////////////////////////////////////////////////////////////////////////////////
// Historical Corn Prices
	router.GET("/corn_prices", func(c *gin.Context) {

		dateStr := c.Query("date")

		if dateStr == "" {
			c.JSON(400, gin.H{"error": "date and source are required"})
			return
		}

		date, err := time.Parse("2006-01-02", dateStr)
		if err != nil {
			c.JSON(400, gin.H{
				"error": "invalid date format, use YYYY-MM-DD",
			})
			return
		}
		
		startDate := time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, time.UTC)
		endDate := startDate.Add(24 * time.Hour)
		data, err := database.QueryCornPrices(startDate, endDate)
		if err != nil {
			c.JSON(500, gin.H{"error": "database query failed"})
			return
		}

		c.JSON(200, data)
	})

///////////////////////////////////////////////////////////////////////////////////////////////////////////////////
// Simulations Results
	router.GET("/simresults", func(c *gin.Context) {
    cellStr := c.Query("cell")
    if cellStr == "" {
        c.JSON(400, gin.H{"error": "cell parameter is required"})
        return
    }

    cellID, err := strconv.Atoi(cellStr)
    if err != nil {
        c.JSON(400, gin.H{"error": "cell must be an integer"})
        return
    }

	// planting_date filter
    plantingDateStr := c.Query("planting_date")
    if plantingDateStr == "" {
        c.JSON(400, gin.H{"error": "planting_date parameter is required"})
        return
    }

    plantingDate, err := strconv.Atoi(plantingDateStr)
    if err != nil {
        c.JSON(400, gin.H{"error": "planting_date must be an integer"})
        return
    }
	// Optional parameters (Nitrogen and grain price)
	nitroPriceStr := c.Query("nitro_price")
	
	var nitroPrice float64

	if nitroPriceStr != "" {
		nitroPrice,err = strconv.ParseFloat(nitroPriceStr,64)
		if err != nil{
			c.JSON(400, gin.H{"error" : "Nitrogen price should be a number (Float)"})
			return
		}
	} else {
		nitroPrice = 0.4
	}

	grainPriceStr := c.Query("grain_price")

	var grainPrice float64

	if grainPriceStr != "" {
		grainPrice,err = strconv.ParseFloat(grainPriceStr,64)
		if err !=nil{
			c.JSON(400, gin.H{"error":"Grain price should be a number (Float)"})
			return
		}
	} else{
		grainPrice = 4
	}
	// Quering database
    sims, err := database.QuerySim(cellID,plantingDate, nitroPrice, grainPrice)
    if err != nil {
        c.JSON(500, gin.H{"error": "database query failed"})
        return
    }

    c.JSON(200, sims)


})
	addr := listenAddr()
	log.Println("Server listening on", addr)
	if err := router.Run(addr); err != nil {
		log.Fatal(err)
	}

}

func listenAddr() string {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	return "0.0.0.0:" + port
}



	


