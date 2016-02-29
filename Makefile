all:
	go test gopkg.in/go-pg/migrations.v4 -cpu=1,2,4
	go test gopkg.in/go-pg/migrations.v4 -short -race
