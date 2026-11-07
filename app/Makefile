.PHONY: web build run check dist test
web:
	cd web && npm install && npx vite build
build: web
	cd server && CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o ../bin/yxz ./cmd/yxz
run: build
	./bin/yxz serve -content content
check:
	cd server && go run ./cmd/yxz check -content ../content -env ../.env
test:
	cd server && go vet ./... && go test ./...
dist: web
	cd server && GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o ../bin/yxz-windows.exe ./cmd/yxz
	cd server && GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o ../bin/yxz-mac-arm64 ./cmd/yxz
	cd server && GOOS=darwin GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o ../bin/yxz-mac-intel ./cmd/yxz
