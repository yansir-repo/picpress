APP_NAME := PicPress
EXEC := picpress
BIN := bin/$(EXEC)
APP := bin/$(APP_NAME).app
GO_TAGS := production,nodynamic
FRONTEND := frontend

.PHONY: build app run dev frontend-install frontend-build test check clean

build: frontend-build
	go build -tags "$(GO_TAGS)" -trimpath -buildvcs=false -ldflags="-s -w" -o "$(BIN)" .

app: build
	rm -rf "$(APP)"
	mkdir -p "$(APP)/Contents/MacOS" "$(APP)/Contents/Resources"
	cp "$(BIN)" "$(APP)/Contents/MacOS/$(EXEC)"
	cp build/darwin/Info.plist "$(APP)/Contents/Info.plist"
	cp build/darwin/icons.icns "$(APP)/Contents/Resources/icons.icns"
	xattr -cr "$(APP)"
	codesign --force --deep --sign - "$(APP)"
	codesign --verify --deep --strict "$(APP)"
	@echo "Built $(APP)"

run: build
	./$(BIN)

dev:
	./scripts/dev.sh

frontend-install:
	pnpm --dir $(FRONTEND) install --frozen-lockfile

frontend-build: frontend-install
	rm -rf $(FRONTEND)/dist
	mkdir -p $(FRONTEND)/dist
	touch $(FRONTEND)/dist/.gitkeep
	pnpm --dir $(FRONTEND) build

test:
	go test ./...
	pnpm --dir $(FRONTEND) exec vp test run

check:
	pnpm --dir $(FRONTEND) check
	go vet ./...

clean:
	rm -rf bin
	rm -rf $(FRONTEND)/dist
	mkdir -p $(FRONTEND)/dist
	touch $(FRONTEND)/dist/.gitkeep
