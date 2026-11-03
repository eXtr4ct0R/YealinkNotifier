package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"github.com/go-toast/toast"
)

var (
	winmm      = syscall.NewLazyDLL("winmm.dll")
	playSoundW = winmm.NewProc("PlaySoundW")
)

const (
	SND_ASYNC    = 0x0001
	SND_FILENAME = 0x00020000

	powershellPath = `C:\Windows\System32\WindowsPowerShell\v1.0\powershell.exe`
)

var fallbackSem = make(chan struct{}, 3)

func playWindowsSound() {
	soundPath := `C:\Windows\Media\Windows Notify System Generic.wav`
	if _, err := os.Stat(soundPath); os.IsNotExist(err) {
		soundPath = `C:\Windows\Media\Windows Notify.wav`
	}

	ptr, err := syscall.UTF16PtrFromString(soundPath)
	if err == nil {
		// Play sound asynchronously
		playSoundW.Call(uintptr(unsafe.Pointer(ptr)), 0, SND_ASYNC|SND_FILENAME)
	}
}

func showToast(title, message string) {
	notification := toast.Notification{
		AppID:   "YealinkNotifier",
		Title:   title,
		Message: message,
	}

	if err := notification.Push(); err != nil {
		log.Printf("Error in go-toast (%v), use PowerShell fallback", err)

		select {
		case fallbackSem <- struct{}{}:
			defer func() { <-fallbackSem }()
		default:
			log.Printf("PowerShell fallback busy, skipping")
			return
		}

		psScript := `
			[Windows.UI.Notifications.ToastNotificationManager, Windows.UI.Notifications, ContentType = WindowsRuntime] > $null
			$template = [Windows.UI.Notifications.ToastNotificationManager]::GetTemplateContent([Windows.UI.Notifications.ToastTemplateType]::ToastText02)
			$textNodes = $template.GetElementsByTagName("text")
			$textNodes.Item(0).AppendChild($template.CreateTextNode($args[0])) > $null
			$textNodes.Item(1).AppendChild($template.CreateTextNode($args[1])) > $null
			$toast = [Windows.UI.Notifications.ToastNotification]::new($template)
			[Windows.UI.Notifications.ToastNotificationManager]::CreateToastNotifier("YealinkNotifier").Show($toast)
		`
		cmd := exec.Command(powershellPath, "-NoProfile", "-Command", psScript, title, message)

		if err := cmd.Run(); err != nil {
			log.Printf("Error in PowerShell (%v)", err)
			return
		}
	}
}

func handler(w http.ResponseWriter, r *http.Request) {
	path := strings.ReplaceAll(strings.ReplaceAll(r.URL.Path, "\r", "\\r"), "\n", "\\n")
	log.Printf("[%s] Signal received: %s", r.RemoteAddr, path)

	query := r.URL.Query()
	caller := query.Get("number")
	if caller == "" {
		caller = query.Get("caller")
	}

	title := "📞 Incoming call from Yealink"
	message := "You are being called! Check your office phone."
	if caller != "" {
		message = fmt.Sprintf("Caller number: %s", caller)
	}

	go showToast(title, message)
	go playWindowsSound()

	w.WriteHeader(http.StatusOK)
	w.Write([]byte("OK"))
}

var version = "dev"

func main() {
	port := flag.Int("port", 8080, "working port")
	ip := flag.String("addr", "0.0.0.0", "bind address")
	versionFlag := flag.Bool("version", false, "show version")

	flag.Parse()
	if *versionFlag {
		fmt.Printf("version: %s\n", version)
		os.Exit(0)
	}

	if *ip == "" || *port == 0 {
		flag.Usage()
		os.Exit(1)
	}

	addr := fmt.Sprintf("%s:%d", *ip, *port)

	http.HandleFunc("/", handler)

	fmt.Println("==================================================")
	fmt.Printf("   Version: %s\n", version)
	fmt.Printf("   Server listening on port: %d\n", *port)
	fmt.Printf("   URL for Yealink Webhook:\n")
	fmt.Printf("   http://%s/?number=$remote_number\n", addr)
	fmt.Printf("   or http://%s/?number=%%remote_number%%\n", addr)
	fmt.Printf("   or http://%s/\n", addr)
	fmt.Println("==================================================")

	srv := &http.Server{
		Addr:              addr,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	if err := srv.ListenAndServe(); err != nil {
		log.Fatalf("Error starting server: %v", err)
	}
}
