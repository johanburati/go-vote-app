package main

import (
	"fmt"
	"net/http"
	"os"

	"github.com/gin-gonic/gin"
	"github.com/gomodule/redigo/redis"
	"github.com/spf13/viper"
)

func main() {

	// use viper to read config

	viper.SetDefault("debug", true)
	viper.SetDefault("port", "8080")
	viper.SetDefault("title", "title default")
	viper.SetDefault("choices", [2]string{"First", "Second"})
	viper.SetDefault("showhost", true)

	viper.SetConfigType("toml")
	viper.SetConfigFile("config.toml")
	err := viper.ReadInConfig()
	if err != nil {
		panic(fmt.Errorf("config: %s\n", err))
	}

	debug := viper.GetBool("debug")
	port := viper.GetString("port")
	title := viper.GetString("title")
	choices := viper.GetStringSlice("choices")
	showhost := viper.GetBool("showhost")

	if debug != true	{ 
		gin.SetMode(gin.ReleaseMode) 
	}

	if showhost == true {
		hostname, err := os.Hostname()
		if err == nil {
			title = hostname
		}
	}

	// set connection to redis db

	pool := redis.NewPool(func() (redis.Conn, error) {
		c, err := redis.Dial("tcp", ":6379")
		
		if err != nil {
			return nil, err
		}

		return c, err
	}, 10)

	defer pool.Close()

	conn := pool.Get()
	defer conn.Close()

	// test connection with a PING

	reply, err := redis.String(conn.Do("PING"))
	if err != nil {
		panic(fmt.Errorf("[REDIS-error] %s\n", err))
	} else {
		fmt.Printf("[REDIS-debug] %s\n", reply)
	}

	// create a map with the different choices and get the votes from the db

	votes := make(map[string]int)
	for i := 0; i < len(choices); i +=1 {
		vote, err := redis.Int(conn.Do("GET", choices[i]))
		if err != nil {
			fmt.Errorf("[REDIS-error] %s\n", err)
		}
		votes[choices[i]] = vote
	}
	fmt.Printf("[REDIS-debug] %v\n", votes)


	// Set the router as the default one shipped with Gin

	router := gin.Default()

	router.LoadHTMLGlob("views/*.html")
	router.GET("/", func(c *gin.Context) {

		// reads the vote count from the db, in case it has been updated on another node

		for k := range votes {
			v, err := redis.Int(conn.Do("GET", k))
			if err != nil {
				fmt.Errorf("[REDIS-error] %s\n", err)
			}
			votes[k] = v
		}

		c.HTML(
			http.StatusOK,
			"index.html",
				gin.H{
					"title": title,
					"votes": votes,
				},
		)
	})

	router.POST("/", func(c *gin.Context) {
		vote := c.PostForm("vote")
		
		// if vote is a key is part of choices, increment

		if _, ok := votes[vote]; ok {

			ret, err :=	conn.Do("INCR", vote)
			if err != nil {
					fmt.Errorf("[REDIS-error] %s\n", err)
			} else {
					fmt.Printf("[REDIS-debug] %s=%i\n", vote, ret)
			}

		}

		// in case vote is reset, reset the votes

		if vote == "reset" {
			for v := range votes {
				_ ,err := conn.Do("SET", v, 0)
				if err != nil {
					fmt.Errorf("[REDIS-error] %s\n", err)
				}
			}
		}

		/* print json
		c.JSON(200, gin.H{
			"status":  "posted",
			"vote": vote,
		}) */


		// reload the main page

		c.Redirect(http.StatusMovedPermanently, "/")

	})

	// Setup r group for the API
	api := router.Group("/api")
	{
		api.GET("/", func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H {
				"message": "pong",
			})
		})
	}

	// Start and run the server
	pport := ":" + port
	router.Run(pport)
}
