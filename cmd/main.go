package main

import (
	"github.com/gin-gonic/gin"
	"github.com/yurin-kami/CloudKaho/config"
	"github.com/yurin-kami/CloudKaho/internal/routes"
)

func main() {
	config.MustLoad()
	// Initialize the server and routes here
	router := gin.Default()
	// Call the function to set up user routes
	routes.UserRoute(router)

	// Start the server
	router.Run(":8080")
}
