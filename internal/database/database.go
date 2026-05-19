package database

import (
	"context"
	"fmt"
	"log"
	"math"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/JorgeJola/indnratebackend/internal/models"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"
	"gonum.org/v1/gonum/mat"
)

var DB *pgxpool.Pool

// Connect opens the pool using DATABASE_URL (preferred on Render) or DB_* vars.
func Connect() error {
	fmt.Fprintln(os.Stderr, "database: loading configuration")
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found, using system environment variables")
	}

	dsn := dsnFromEnv()
	if dsn == "" {
		return fmt.Errorf("set DATABASE_URL or DB_HOST, DB_PORT, DB_NAME, DB_USER, DB_PASSWORD (and ensure the Postgres instance is linked or vars are set in Render)")
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return fmt.Errorf("create pool: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		return fmt.Errorf(`database ping failed: %w

Fix on Render: (1) Postgres → Connections → copy the Internal Database URL into DATABASE_URL on this Web Service (same region as the DB). External hostnames often fail from the private network. (2) If you still see TLS errors on an older Postgres version, append sslnegotiation=postgres to DATABASE_URL to override direct TLS.`, err)
	}
	DB = pool
	log.Println("Connected to PostgreSQL (ping OK)")
	return nil
}

func dsnFromEnv() string {
	if u := strings.TrimSpace(os.Getenv("DATABASE_URL")); u != "" {
		return ensureSSLMode(u)
	}
	return buildDSNFromParts()
}

// ensureSSLMode forces sslmode=require. Render URLs often use sslmode=prefer, which can
// trigger plaintext startup and "unexpected EOF" against servers that require TLS.
func ensureSSLMode(conn string) string {
	u, err := url.Parse(conn)
	if err != nil {
		return conn + "?sslmode=require"
	}
	if u.Scheme != "postgres" && u.Scheme != "postgresql" {
		return conn
	}
	q := u.Query()
	if q.Get("sslmode") == "disable" {
		return conn
	}
	q.Set("sslmode", "require")
	q.Set("connect_timeout", "15")
	// Render and some cloud Postgres endpoints close with "unexpected EOF" when using the
	// legacy SSLRequest handshake; direct TLS (PostgreSQL 17+ / pgx sslnegotiation) avoids it.
	if q.Get("sslnegotiation") == "" {
		q.Set("sslnegotiation", "direct")
	}
	u.RawQuery = q.Encode()
	return u.String()
}

func buildDSNFromParts() string {
	host := strings.TrimSpace(os.Getenv("DB_HOST"))
	port := strings.TrimSpace(os.Getenv("DB_PORT"))
	user := os.Getenv("DB_USER")
	pass := os.Getenv("DB_PASSWORD")
	name := strings.TrimSpace(os.Getenv("DB_NAME"))
	if host == "" || port == "" || name == "" || user == "" {
		return ""
	}
	u := &url.URL{
		Scheme: "postgres",
		User:   url.UserPassword(user, pass),
		Host:   fmt.Sprintf("%s:%s", host, port),
		Path:   "/" + name,
	}
	q := u.Query()
	q.Set("sslmode", "require")
	q.Set("connect_timeout", "15")
	if q.Get("sslnegotiation") == "" {
		q.Set("sslnegotiation", "direct")
	}
	u.RawQuery = q.Encode()
	return u.String()
}
func fitPoly2(x, y []float64) []float64 {

	n := len(x)

	X := mat.NewDense(n, 3, nil)
	Y := mat.NewDense(n, 1, nil)

	for i := 0; i < n; i++ {
		X.Set(i, 0, 1)
		X.Set(i, 1, x[i])
		X.Set(i, 2, x[i]*x[i])

		Y.Set(i, 0, y[i])
	}

	var xt mat.Dense
	xt.Mul(X.T(), X)

	var xty mat.Dense
	xty.Mul(X.T(), Y)

	var coef mat.Dense
	coef.Solve(&xt, &xty)

	return []float64{
		coef.At(0, 0),
		coef.At(1, 0),
		coef.At(2, 0),
	}
}

func evalPoly2(coef []float64, x float64) float64 {
		return coef[0] + coef[1]*x + coef[2]*x*x
	}

func QuerySim(cellID int, plantingDate int, nitroPrice float64, grainPrice float64) ([]models.Simulation, error) {

	rows, err := DB.Query(context.Background(),
		`SELECT nitro_kg_ha, yield_kg_ha 
		 FROM simulations 
		 WHERE id_cell=$1 AND planting_date=$2`,
		cellID, plantingDate)
		
	if err != nil {
		log.Printf("database query error: %v", err)
		return nil, err
	}
	defer rows.Close()

	// group by nitrogen (lb/ac)
	group := make(map[float64][]float64)

	for rows.Next() {

		var nitroKg, yieldKg float64

		if err := rows.Scan(&nitroKg, &yieldKg); err != nil {
			log.Println("Row scan error:", err)
			continue
		}

		nitroLb := nitroKg * 0.892
		yieldBu := yieldKg / 62.77


		profit := (yieldBu * grainPrice) - (nitroLb * nitroPrice)


		nitroLb = math.Round(nitroLb*100) / 100

		group[nitroLb] = append(group[nitroLb], profit)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	var x []float64
	var y []float64

	for nitro, profits := range group {

		var sum float64
		for _, p := range profits {
			sum += p
		}

		avg := sum / float64(len(profits))

		x = append(x, nitro)
		y = append(y, avg)
	}

	coef := fitPoly2(x, y)

	var result []models.Simulation
	
	for n := 0.0; n <= 268.0; n += 1 {

		p := evalPoly2(coef, n)

		result = append(result, models.Simulation{
			NitroLbAc: n,
			ProfitDol: p,
		})
	}

	return result, nil
}

func QueryEonrCount(regionID string, nitroPrice float64, grainPrice float64) ([]models.Eonr, error) {
	rows, err := DB.Query(context.Background(),
		"SELECT id_trial, nitro_kg_ha, yield_kg_ha, id_region FROM on_farm WHERE id_region=$1", regionID)
	if err != nil {
		log.Printf("database query error: %v", err)
		return nil, err
	}
	defer rows.Close()


	trials := make(map[string][]struct {
		N float64 
		Y float64 
		R string
	})

	for rows.Next() {
		var idTrial string
		var nitroKgHa, yieldKgHa float64
		var region string

		if err := rows.Scan(&idTrial, &nitroKgHa, &yieldKgHa, &region); err != nil {
			log.Println("Row scan error:", err)
			continue
		}


		nitroLbAc := nitroKgHa * 0.892
		yieldBsAc := yieldKgHa / 62.77

		trials[idTrial] = append(trials[idTrial], struct {
			N float64
			Y float64
			R string
		}{nitroLbAc, yieldBsAc, region})
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	var results []models.Eonr
	// Calculates EONR iterating through all trials
	for idTrial, data := range trials {
		maxProfit := -1e18
		eonr := 0.0
		region := data[0].R

		for _, d := range data {
			profit := d.Y*grainPrice - d.N*nitroPrice 

			if profit > maxProfit {
				maxProfit = profit
				eonr = d.N
			} else if profit == maxProfit && d.N < eonr {
				eonr = d.N
			}
		}

		results = append(results, models.Eonr{
			IDTrial: idTrial,
			Region:  region,
			EONR:    eonr,     
			Profit:  maxProfit, 
		})
	}

	return results, nil
}

func QueryNitroPrices(startDate, endDate time.Time, source string) ([]models.NitroPrice, error) {

	query := `
		SELECT date, nitro_source, nitro_price_lb
		FROM nitro_prices
		WHERE date >= $1 AND date < $2
		AND nitro_source = $3
		ORDER BY date ASC
	`

	rows, err := DB.Query(context.Background(), query, startDate, endDate, source)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []models.NitroPrice

	for rows.Next() {
		var np models.NitroPrice

		if err := rows.Scan(
			&np.Date,
			&np.NitroSource,
			&np.NitroPriceLb,
		); err != nil {
			return nil, err
		}

		results = append(results, np)
	}

	return results, rows.Err()
}

func QueryCornPrices(startDate, endDate time.Time) ([]models.CornPrice, error) {

	query := `
		SELECT date, corn_price_bu
		FROM corn_prices
		WHERE date >= $1 AND date < $2
		ORDER BY date ASC
	`

	rows, err := DB.Query(context.Background(), query, startDate, endDate)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []models.CornPrice

	for rows.Next() {
		var np models.CornPrice

		if err := rows.Scan(
			&np.Date,
			&np.CornPriceLb,
		); err != nil {
			return nil, err
		}

		results = append(results, np)
	}

	return results, rows.Err()
}