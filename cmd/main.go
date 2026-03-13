package cmd

import (
	"github.com/gin-gonic/gin"
	"github.com/yurin-kami/CloudKaho/internal/routes"
)

func main() {
	// Initialize the server and routes here
	router := gin.Default()
	// Call the function to set up user routes
	routes.UserRoute(router)

	// Start the server
	router.Run(":8080")
}
