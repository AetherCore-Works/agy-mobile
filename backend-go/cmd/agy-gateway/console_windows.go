//go:build windows

package main

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"github.com/Phoenix7290/agy-mobile-gateway/backend-go/internal/config"
	"golang.org/x/sys/windows"
)

const createNoWindowFlag = 0x08000000

var (
	kernel32DLL         = windows.NewLazySystemDLL("kernel32.dll")
	procSetConsoleTitle = kernel32DLL.NewProc("SetConsoleTitleW")
	procSetOutputCP     = kernel32DLL.NewProc("SetConsoleOutputCP")
)

func setConsoleWindowTitle(title string) {
	if ptr, err := windows.UTF16PtrFromString(title); err == nil {
		_, _, _ = procSetConsoleTitle.Call(uintptr(unsafe.Pointer(ptr)))
	}
}

func maybeRunConsoleController() bool {
	args := os.Args[1:]
	if slices.Contains(args, "--daemon") || envTruthy(os.Getenv("AGY_HEADLESS")) {
		return false
	}
	forceConsole := slices.Contains(args, "--console")
	if !forceConsole && !isInteractiveConsole() {
		return false
	}
	cfg := config.Load()
	if isGatewayHealthyLocal(cfg.Port) && strings.TrimSpace(os.Getenv("NO_SINGLE_INSTANCE_OPEN")) == "" {
		_, _, _ = procSetOutputCP.Call(65001)
		fmt.Printf("AGY Mobile Gateway já está rodando em http://127.0.0.1:%s. Abrindo no navegador...\n", cfg.Port)
		openGatewayAppWindow(cfg.Port)
		time.Sleep(1200 * time.Millisecond)
		return true
	}
	runWindowsConsoleUI(cfg)
	return true
}

func envTruthy(raw string) bool {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func isInteractiveConsole() bool {
	fi, err := os.Stdin.Stat()
	if err != nil || (fi.Mode()&os.ModeCharDevice) == 0 {
		return false
	}
	var mode uint32
	if err := windows.GetConsoleMode(windows.Handle(os.Stdin.Fd()), &mode); err != nil {
		return false
	}
	var outMode uint32
	if err := windows.GetConsoleMode(windows.Handle(os.Stdout.Fd()), &outMode); err != nil {
		return false
	}
	return true
}

type supervisedChild struct {
	name string
	cmd  *exec.Cmd
	logF *os.File
}

type consoleSupervisor struct {
	cfg     config.GatewaySettings
	workDir string
	exePath string
	logDir  string
	job     windows.Handle

	mu          sync.Mutex
	children    []*supervisedChild
	lastNotice  string
	gatewayDown bool
}

func newConsoleSupervisor(cfg config.GatewaySettings) *consoleSupervisor {
	exe, err := os.Executable()
	if err != nil {
		exe = "agy-gateway.exe"
	}
	job, _ := createKillOnCloseJob()
	return &consoleSupervisor{
		cfg:     cfg,
		workDir: resolveConsoleWorkDir(cfg.Dirs.DataDir),
		exePath: exe,
		logDir:  filepath.Join(cfg.Dirs.DataDir, "logs"),
		job:     job,
	}
}

func createKillOnCloseJob() (windows.Handle, error) {
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return 0, err
	}
	var info windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION
	info.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
	if _, err := windows.SetInformationJobObject(
		job,
		windows.JobObjectExtendedLimitInformation,
		uintptr(unsafe.Pointer(&info)),
		uint32(unsafe.Sizeof(info)),
	); err != nil {
		_ = windows.CloseHandle(job)
		return 0, err
	}
	return job, nil
}

func (s *consoleSupervisor) assignJob(cmd *exec.Cmd) {
	if s.job == 0 || cmd == nil || cmd.Process == nil {
		return
	}
	h, err := windows.OpenProcess(windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE, false, uint32(cmd.Process.Pid))
	if err != nil {
		return
	}
	defer func() { _ = windows.CloseHandle(h) }()
	_ = windows.AssignProcessToJobObject(s.job, h)
}

func (s *consoleSupervisor) closeJob() {
	if s.job != 0 {
		_ = windows.CloseHandle(s.job)
		s.job = 0
	}
}

func (s *consoleSupervisor) startAll() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if isGatewayHealthyLocal(s.cfg.Port) && len(s.children) == 0 {
		s.lastNotice = fmt.Sprintf("Gateway já estava ativo em :%s (aberto externamente).", s.cfg.Port)
		return
	}

	_ = os.MkdirAll(s.logDir, 0o755)
	exeDir := filepath.Dir(s.exePath)

	// 1. Optional ttyd terminal
	if enabled, _ := s.cfg.Mods["terminal"].(bool); enabled && !isLocalPortOpen(7681) {
		if ttydBin, err := exec.LookPath("ttyd"); err == nil {
			shellExe := "powershell.exe"
			if pwsh, err := exec.LookPath("pwsh"); err == nil {
				shellExe = pwsh
			}
			_ = s.spawnLocked("ttyd", ttydBin, []string{
				"-p", "7681", "-b", "/terminal", "-W", "-t", "fontSize=14",
				"-w", s.workDir, shellExe, "-NoLogo",
			}, s.workDir, nil, filepath.Join(s.logDir, "ttyd.log"))
		}
	}

	// 2. Optional Cockpit daemon
	if enabled, _ := s.cfg.Mods["cockpit"].(bool); enabled && !isLocalPortOpen(8099) {
		cockpitCandidates := []string{
			filepath.Join(exeDir, "agy-cockpit.exe"),
			filepath.Join(s.workDir, "packages", "cockpit", "agy-cockpit.exe"),
		}
		for _, c := range cockpitCandidates {
			if _, err := os.Stat(c); err == nil {
				extraEnv := []string{
					"COCKPIT_ADDR=127.0.0.1:8099",
					"COCKPIT_ALLOW_POWER=true",
				}
				_ = s.spawnLocked("agy-cockpit", c, nil, exeDir, extraEnv, filepath.Join(s.logDir, "cockpit.log"))
				break
			}
		}
	}

	// 3. Gateway daemon
	if !isLocalPortOpenStr(s.cfg.Port) {
		extraEnv := []string{"AGY_HEADLESS=1"}
		if err := s.spawnLocked("agy-gateway", s.exePath, []string{"--daemon"}, s.workDir, extraEnv, filepath.Join(s.logDir, "gateway.err.log")); err != nil {
			s.lastNotice = "Erro ao iniciar Gateway: " + err.Error()
			return
		}
		// Wait briefly for HTTP listener readiness
		ready := false
		for range 25 {
			if isGatewayHealthyLocal(s.cfg.Port) {
				ready = true
				break
			}
			time.Sleep(120 * time.Millisecond)
		}
		if ready {
			s.lastNotice = "AGY iniciado e pronto para uso."
		} else {
			s.lastNotice = "Gateway iniciado (aguardando resposta HTTP...)."
		}
	} else {
		s.lastNotice = "Serviços já estavam em execução."
	}
}

func (s *consoleSupervisor) spawnLocked(name, bin string, args []string, dir string, extraEnv []string, logPath string) error {
	for _, c := range s.children {
		if c.name == name && c.cmd != nil && c.cmd.ProcessState == nil {
			return nil
		}
	}
	f, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		f = nil
	}
	cmd := exec.Command(bin, args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), extraEnv...)
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: createNoWindowFlag}
	if f != nil {
		cmd.Stdout = f
		cmd.Stderr = f
	}
	if err := cmd.Start(); err != nil {
		if f != nil {
			_ = f.Close()
		}
		return err
	}
	s.assignJob(cmd)

	child := &supervisedChild{name: name, cmd: cmd, logF: f}
	s.children = append(s.children, child)

	go func(ch *supervisedChild) {
		_ = cmd.Wait()
		if ch.logF != nil {
			_ = ch.logF.Close()
		}
	}(child)
	return nil
}

func (s *consoleSupervisor) stopAll() {
	s.mu.Lock()
	children := s.children
	s.children = nil
	s.mu.Unlock()

	for _, ch := range children {
		if ch.cmd != nil && ch.cmd.Process != nil {
			_ = ch.cmd.Process.Kill()
		}
		if ch.logF != nil {
			_ = ch.logF.Close()
		}
	}
	time.Sleep(180 * time.Millisecond)
	if isLocalPortOpenStr(s.cfg.Port) || isLocalPortOpen(8099) || isLocalPortOpen(7681) {
		_ = exec.Command("powershell.exe", "-NoProfile", "-WindowStyle", "Hidden", "-Command",
			"$p8088=Get-NetTCPConnection -LocalPort "+s.cfg.Port+" -State Listen -EA SilentlyContinue | Select -First 1 -Expand OwningProcess; if($p8088){Stop-Process -Id $p8088 -Force -EA SilentlyContinue}; "+
				"$p8099=Get-NetTCPConnection -LocalPort 8099 -State Listen -EA SilentlyContinue | Select -First 1 -Expand OwningProcess; if($p8099){Stop-Process -Id $p8099 -Force -EA SilentlyContinue}; "+
				"$p7681=Get-NetTCPConnection -LocalPort 7681 -State Listen -EA SilentlyContinue | Select -First 1 -Expand OwningProcess; if($p7681){Stop-Process -Id $p7681 -Force -EA SilentlyContinue}",
		).Run()
		time.Sleep(150 * time.Millisecond)
	}
	s.mu.Lock()
	s.lastNotice = "Tudo encerrado. Modo privacidade ativo."
	s.mu.Unlock()
}

func (s *consoleSupervisor) statusNotice() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.lastNotice
}

func runWindowsConsoleUI(cfg config.GatewaySettings) {
	_, _, _ = procSetOutputCP.Call(65001)
	setConsoleWindowTitle("AGY Mobile Gateway - Painel de Controle")

	var origOutMode uint32
	outHandle := windows.Handle(os.Stdout.Fd())
	if windows.GetConsoleMode(outHandle, &origOutMode) == nil {
		_ = windows.SetConsoleMode(outHandle, origOutMode|windows.ENABLE_VIRTUAL_TERMINAL_PROCESSING)
	}

	sup := newConsoleSupervisor(cfg)
	defer func() {
		sup.stopAll()
		sup.closeJob()
	}()

	sigCh := make(chan os.Signal, 2)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)

	// Start immediately when user clicks to open AGY
	sup.startAll()

	keyCh := startConsoleKeyReader()
	ticker := time.NewTicker(4 * time.Second)
	defer ticker.Stop()

	renderConsoleScreen(sup)

	for {
		select {
		case <-sigCh:
			fmt.Println("\nEncerrando serviços e fechando...")
			return
		case ch, ok := <-keyCh:
			if !ok {
				return
			}
			switch strings.ToLower(string(ch)) {
			case "1", "l":
				sup.startAll()
				renderConsoleScreen(sup)
			case "2", "d":
				sup.stopAll()
				renderConsoleScreen(sup)
			case "3", "r":
				sup.stopAll()
				sup.startAll()
				renderConsoleScreen(sup)
			case "4", "o":
				if isGatewayHealthyLocal(cfg.Port) {
					openGatewayAppWindow(cfg.Port)
					sup.mu.Lock()
					sup.lastNotice = fmt.Sprintf("Navegador aberto em http://127.0.0.1:%s", cfg.Port)
					sup.mu.Unlock()
				} else {
					sup.mu.Lock()
					sup.lastNotice = "Gateway desligado. Pressione [1] para ligar primeiro."
					sup.mu.Unlock()
				}
				renderConsoleScreen(sup)
			case "5":
				showLogTail(filepath.Join(sup.logDir, "gateway.err.log"), 14)
				fmt.Print("\nPressione qualquer tecla para voltar ao painel...")
				<-keyCh
				renderConsoleScreen(sup)
			case "6":
				renderConsoleScreen(sup)
			case "q", "\x1b":
				fmt.Println("\nDesligando AGY e fechando terminal...")
				return
			}
		case <-ticker.C:
			renderConsoleScreen(sup)
		}
	}
}

func startConsoleKeyReader() <-chan byte {
	ch := make(chan byte, 16)
	inHandle := windows.Handle(os.Stdin.Fd())
	var origMode uint32
	rawEnabled := false
	if windows.GetConsoleMode(inHandle, &origMode) == nil {
		rawMode := origMode &^ (windows.ENABLE_LINE_INPUT | windows.ENABLE_ECHO_INPUT)
		if windows.SetConsoleMode(inHandle, rawMode) == nil {
			rawEnabled = true
		}
	}

	go func() {
		defer close(ch)
		if rawEnabled {
			defer func() { _ = windows.SetConsoleMode(inHandle, origMode) }()
		}
		buf := make([]byte, 1)
		for {
			n, err := os.Stdin.Read(buf)
			if err != nil || n == 0 {
				return
			}
			ch <- buf[0]
		}
	}()
	return ch
}

func renderConsoleScreen(sup *consoleSupervisor) {
	gwUp := isGatewayHealthyLocal(sup.cfg.Port)
	cockpitUp := isLocalPortOpen(8099)
	ttydUp := isLocalPortOpen(7681)
	lanIPs := detectLANIPs()

	fmt.Print("\033[2J\033[H")
	fmt.Println("================================================================")
	fmt.Println("  AGY Mobile Gateway - Controle de Execução e Privacidade")
	fmt.Println("================================================================")
	if gwUp {
		fmt.Println("  Status Geral : \033[32m[ RODANDO ]\033[0m  (Fechar no X desliga tudo)")
	} else {
		fmt.Println("  Status Geral : \033[31m[ DESLIGADO ]\033[0m  (Privacidade ativa)")
	}
	fmt.Printf("  Workspace    : %s\n", sup.workDir)
	fmt.Println("----------------------------------------------------------------")
	fmt.Printf("  Gateway (:%s) : %s\n", sup.cfg.Port, formatOnOff(gwUp))
	fmt.Printf("  Cockpit (:8099) : %s\n", formatOnOff(cockpitUp))
	fmt.Printf("  Terminal(:7681) : %s\n", formatOnOff(ttydUp))
	fmt.Println("----------------------------------------------------------------")
	if gwUp {
		fmt.Printf("  Abrir no PC      : http://127.0.0.1:%s\n", sup.cfg.Port)
		if len(lanIPs) > 0 {
			fmt.Printf("  Abrir no Celular : http://%s:%s\n", lanIPs[0], sup.cfg.Port)
			for _, extra := range lanIPs[1:] {
				fmt.Printf("  Outro IP / VPN   : http://%s:%s\n", extra, sup.cfg.Port)
			}
		} else {
			fmt.Println("  Abrir no Celular : (nenhum IP LAN detectado)")
		}
	} else {
		fmt.Println("  Nenhuma porta do AGY está ativa neste momento.")
	}
	fmt.Println("----------------------------------------------------------------")
	fmt.Println("  [1] Ligar tudo            [2] Desligar tudo (Privacidade)")
	fmt.Println("  [3] Reiniciar             [4] Abrir no Navegador")
	fmt.Println("  [5] Ver log recente       [6] Atualizar status")
	fmt.Println("  [Q] Desligar e Fechar terminal")
	fmt.Println("================================================================")
	if msg := sup.statusNotice(); msg != "" {
		fmt.Printf("  Info: %s\n", msg)
	}
}

func formatOnOff(up bool) string {
	if up {
		return "\033[32mON\033[0m"
	}
	return "\033[90mOFF\033[0m"
}

func isLocalPortOpenStr(portStr string) bool {
	p, err := strconv.Atoi(strings.TrimSpace(portStr))
	if err != nil || p <= 0 {
		return false
	}
	return isLocalPortOpen(p)
}

func isLocalPortOpen(port int) bool {
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), 180*time.Millisecond)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}

func resolveConsoleWorkDir(fallback string) string {
	if envDir := strings.TrimSpace(os.Getenv("AGY_WORKSPACE")); envDir != "" {
		if info, err := os.Stat(envDir); err == nil && info.IsDir() {
			return envDir
		}
	}
	if wd, err := os.Getwd(); err == nil {
		lower := strings.ToLower(wd)
		if !strings.Contains(lower, `\windows\system32`) && !strings.Contains(lower, `\appdata\local\agy\bin`) {
			return wd
		}
	}
	return fallback
}

func detectLANIPs() []string {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil
	}
	type candidate struct {
		ip       string
		priority int
	}
	var found []candidate
	for _, iface := range ifaces {
		if (iface.Flags&net.FlagUp) == 0 || (iface.Flags&net.FlagLoopback) != 0 {
			continue
		}
		nameLower := strings.ToLower(iface.Name)
		if strings.Contains(nameLower, "wsl") ||
			strings.Contains(nameLower, "vethernet") ||
			strings.Contains(nameLower, "bluetooth") ||
			strings.Contains(nameLower, "virtual") ||
			strings.Contains(nameLower, "vmware") ||
			strings.Contains(nameLower, "vbox") ||
			strings.Contains(nameLower, "proton") ||
			strings.Contains(nameLower, "openvpn") {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, a := range addrs {
			ipNet, ok := a.(*net.IPNet)
			if !ok {
				continue
			}
			v4 := ipNet.IP.To4()
			if v4 == nil || v4.IsLoopback() || v4.IsLinkLocalUnicast() {
				continue
			}
			ipStr := v4.String()
			prio := 50
			if strings.HasPrefix(ipStr, "192.168.") {
				prio = 10
			} else if strings.HasPrefix(ipStr, "10.") {
				prio = 20
			} else if strings.HasPrefix(ipStr, "172.") {
				prio = 30
			} else if strings.Contains(nameLower, "radmin") || strings.Contains(nameLower, "tailscale") {
				prio = 40
			}
			found = append(found, candidate{ip: ipStr, priority: prio})
		}
	}
	sort.Slice(found, func(i, j int) bool {
		if found[i].priority != found[j].priority {
			return found[i].priority < found[j].priority
		}
		return found[i].ip < found[j].ip
	})
	var out []string
	seen := map[string]bool{}
	for _, c := range found {
		if !seen[c.ip] {
			seen[c.ip] = true
			out = append(out, c.ip)
		}
	}
	return out
}

func showLogTail(path string, maxLines int) {
	fmt.Print("\033[2J\033[H")
	fmt.Printf("=== Log recente (%s) ===\n\n", path)
	f, err := os.Open(path)
	if err != nil {
		fmt.Printf("(sem log disponível: %v)\n", err)
		return
	}
	defer f.Close()

	data, err := io.ReadAll(io.LimitReader(f, 128*1024))
	if err != nil {
		return
	}
	var lines []string
	sc := bufio.NewScanner(strings.NewReader(string(data)))
	for sc.Scan() {
		lines = append(lines, sc.Text())
	}
	if len(lines) > maxLines {
		lines = lines[len(lines)-maxLines:]
	}
	if len(lines) == 0 {
		fmt.Println("(arquivo de log vazio)")
		return
	}
	for _, l := range lines {
		fmt.Println(l)
	}
}
