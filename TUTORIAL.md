# آموزش عمیق Go با تحلیل کد Port Forward Manager

## 📋 فهرست مطالب
1. [معماری کلی پروژه](#معماری-کلی)
2. [تحلیل هر فایل](#تحلیل-فایل-ها)
3. [مفاهیم پیشرفته Go](#مفاهیم-go)
4. [بهترین شیوه‌ها](#بهترین-شیوه-ها)
5. [نکات امنیتی](#امنیت)

---

## معماری کلی

```
go-port-forward/
├── main.go          → Entry point و logic اصلی CLI
├── types.go         → تعریف ساختارها و متغیرهای global
├── storage.go       → مدیریت ذخیره‌سازی (JSON)
├── service.go       → منطق اجرای سرویس‌ها
├── ui.go            → نمایش UI و status monitor
└── colors.go        → رنگ‌های terminal
```

### جریان اجرا:
```
main()
  → دریافت آرگومان‌های CLI
  → switch بر اساس دستور (add/list/run/delete)
  → برای run:
      • راه‌اندازی goroutine برای هر سرویس
      • راه‌اندازی UI loop
      • block کردن با select{}
```

---

## تحلیل فایل‌ها

### 1️⃣ `types.go` - تعریف ساختارها

```go
package main

import "sync"

const DataFile = "services.json"

type ServiceStatus struct {
    Name   string
    Status string
    Local  string
    Remote string
}

var (
    mu       sync.Mutex
    statuses = make(map[string]ServiceStatus)
)
```

#### مفاهیم کلیدی:

**🔸 Package:**
- هر فایل Go باید در یک package باشه
- `package main` = برنامه قابل اجرا (executable)
- فایل‌های دیگه می‌تونن package دیگه‌ای داشته باشن

**🔸 Struct:**
```go
type ServiceStatus struct {
    Name   string  // فیلد public (حرف بزرگ)
    Status string
}
```
- مثل class در زبان‌های دیگه (ولی بدون inheritance)
- فیلدهایی که با حرف بزرگ شروع شن = **Public** (exportable)
- فیلدهایی که با حرف کوچیک شروع شن = **Private**

**🔸 sync.Mutex:**
```go
var mu sync.Mutex
```
- برای **thread-safety** در goroutine‌ها
- وقتی چند goroutine به یک map یا متغیر دسترسی دارن، باید از mutex استفاده کنی
- `mu.Lock()` = قفل کن (فقط یک goroutine می‌تونه بیاد داخل)
- `mu.Unlock()` = باز کن

**🔸 Map:**
```go
statuses = make(map[string]ServiceStatus)
```
- Key-Value storage
- `make()` برای initialize کردن map، slice، channel
- thread-safe نیست → نیاز به mutex

---

### 2️⃣ `colors.go` - رنگ‌های Terminal

```go
const (
    ColorReset  = "\033[0m"
    ColorRed    = "\033[31m"
    ColorGreen  = "\033[32m"
    // ...
)
```

#### مفاهیم:

**🔸 ANSI Escape Codes:**
- `\033[31m` = رنگ قرمز
- `\033[0m` = reset رنگ
- کار می‌کنه در Linux/macOS terminals و Windows 10+ CMD

**🔸 Constants:**
- با `const` تعریف می‌شن
- immutable (غیرقابل تغییر)
- compile-time evaluation

---

### 3️⃣ `storage.go` - مدیریت فایل JSON

```go
func getDataFilePath() string {
    exe, err := os.Executable()
    if err != nil {
        return DataFile
    }
    exeDir := filepath.Dir(exe)
    return filepath.Join(exeDir, DataFile)
}
```

#### مفاهیم:

**🔸 Error Handling در Go:**
```go
exe, err := os.Executable()
if err != nil {
    // مدیریت خطا
    return DataFile
}
```
- Go نداره try/catch
- اکثر توابع یک جفت return دارن: `(result, error)`
- باید همیشه error رو چک کنی

**⚠️ مشکل این کد:**
```go
data, _ := os.ReadFile(dataFile)  // ← نادیده گرفتن error
json.Unmarshal(data, &services)   // ← نادیده گرفتن error
```

**✅ روش بهتر:**
```go
func LoadServices() (map[string]string, error) {
    services := make(map[string]string)
    dataFile := getDataFilePath()

    if _, err := os.Stat(dataFile); os.IsNotExist(err) {
        return services, nil  // فایل نیست، خطا نیست
    }

    data, err := os.ReadFile(dataFile)
    if err != nil {
        return nil, fmt.Errorf("failed to read file: %w", err)
    }

    if err := json.Unmarshal(data, &services); err != nil {
        return nil, fmt.Errorf("failed to parse JSON: %w", err)
    }

    return services, nil
}
```

**🔸 JSON Marshaling:**
```go
data, _ := json.MarshalIndent(services, "", "  ")
```
- `MarshalIndent` = تبدیل struct به JSON با فرمت زیبا
- `""` = prefix
- `"  "` = indent (2 فاصله)

**🔸 Pointers:**
```go
json.Unmarshal(data, &services)
       تبدیل JSON به ←  این map رو تغییر بده
```
- `&` = آدرس متغیر (pointer)
- Unmarshal نیاز داره که map رو تغییر بده، نه یک کپی ازش

---

### 4️⃣ `main.go` - منطق اصلی

#### **الف) Regex برای استخراج پورت‌ها**

```go
func extractPorts(command string) (local, remote string, ok bool) {
    portRegex := regexp.MustCompile(`(\d+):(\d+)`)
    matches := portRegex.FindStringSubmatch(command)
    if len(matches) == 3 {
        return matches[2], matches[1], true
    }
    return "", "", false
}
```

**🔸 Multiple Return Values:**
- Go اجازه میده چند مقدار return کنی
- Pattern رایج: `(result, ok bool)` یا `(result, error)`

**🔸 Regular Expression:**
- `\d+` = یک یا چند رقم
- `(\d+)` = capture group
- `matches[0]` = کل match
- `matches[1]` = اولین گروه
- `matches[2]` = دومین گروه

مثال:
```
input: "kubectl port-forward 8080:5432"
matches[0] = "8080:5432"
matches[1] = "8080"  ← remote
matches[2] = "5432"  ← local
```

**❓ چرا return `matches[2], matches[1]`?**
چون می‌خوایم local رو اول برگردونیم.

---

#### **ب) تابع main() و CLI Parsing**

```go
func main() {
    if len(os.Args) < 2 {
        PrintHelp()
        return
    }

    services := LoadServices()
    cmd := os.Args[1]

    switch cmd {
    case "a", "add":
        // ...
    case "l", "list":
        // ...
    }
}
```

**🔸 os.Args:**
```bash
pf add db "kubectl ..."
↓
os.Args[0] = "pf"
os.Args[1] = "add"
os.Args[2] = "db"
os.Args[3] = "kubectl ..."
```

**🔸 Switch Statement:**
- می‌تونی چند case با هم بذاری: `case "a", "add":`
- نیازی به `break` نیست (پیش‌فرض break داره)
- اگه می‌خوای به case بعدی بره: `fallthrough`

---

#### **ج) دستور Run - Goroutines**

```go
case "r", "run":
    // ...
    for _, name := range names {
        name = strings.TrimSpace(name)
        if command, ok := services[name]; ok {
            local, remote, portOk := extractPorts(command)
            if !portOk {
                continue
            }
            go RunLoop(name, command, local, remote)  // ← Goroutine!
            validServices++
        }
    }

    if validServices > 0 {
        go DisplayStatusLoop()  // ← Goroutine!
        select {}  // ← Block forever
    }
```

**🔸 Goroutines:**
```go
go RunLoop(...)  // راه‌اندازی یک thread جدید
```
- Lightweight thread (خیلی سبک‌تر از OS threads)
- Go runtime مدیریتشون می‌کنه
- Non-blocking: بعد از `go` بلافاصله به خط بعدی میره

**🔸 select {}:**
```go
select {}  // block forever
```
- بدون این، برنامه بلافاصله exit می‌کنه
- goroutine‌ها در background دارن کار می‌کنن
- این باعث میشه main thread منتظر بمونه

**💡 چرا نیاز داریم؟**
```go
go RunLoop(...)       // راه‌اندازی goroutine
// اگه select {} نذاریم:
}  // ← main() تموم میشه، برنامه exit می‌کنه، goroutine‌ها kill میشن
```

---

#### **د) Map Operations**

```go
// خواندن
command, ok := services[name]
if !ok {
    fmt.Println("not found")
}

// نوشتن
services[name] = command

// حذف
delete(services, name)

// حلقه روی map
for name, command := range services {
    fmt.Println(name, command)
}
```

**🔸 Comma-ok pattern:**
```go
value, ok := map[key]
```
- `ok` = true اگه key وجود داشته باشه
- جلوگیری از panic

---

### 5️⃣ `service.go` - اجرای سرویس‌ها

```go
func RunLoop(name, command, localPort, remotePort string) {
    for {
        mu.Lock()
        statuses[name] = ServiceStatus{
            Name:   name,
            Status: "CONNECTING",
            // ...
        }
        mu.Unlock()

        var cmd *exec.Cmd
        if runtime.GOOS == "windows" {
            cmd = exec.Command("cmd", "/C", command)
        } else {
            cmd = exec.Command("bash", "-c", command)
        }

        err := cmd.Start()
        if err != nil {
            // ...
            time.Sleep(500 * time.Millisecond)
            continue
        }

        mu.Lock()
        statuses[name].Status = "ONLINE"
        mu.Unlock()

        cmd.Wait()  // منتظر بمون تا process تموم شه

        // ...
        time.Sleep(500 * time.Millisecond)
    }
}
```

#### مفاهیم:

**🔸 Infinite Loop:**
```go
for {
    // تا ابد
}
```

**🔸 exec.Command:**
```go
cmd := exec.Command("bash", "-c", "kubectl port-forward ...")
```
- اجرای دستورات سیستمی
- **امنیت:** مراقب command injection باش!

**🔸 Cross-platform:**
```go
if runtime.GOOS == "windows" {
    // Windows
} else {
    // Linux/macOS
}
```

**🔸 cmd.Start() vs cmd.Run():**
- `Start()`: شروع کن و برگرد (non-blocking)
- `Run()`: شروع کن و منتظر بمون (blocking)
- `Wait()`: منتظر process که start شده

**🔸 Mutex Pattern:**
```go
mu.Lock()
// تغییر shared data
statuses[name] = ...
mu.Unlock()
```
- همیشه بعد از Lock باید Unlock کنی
- اگه فراموش کنی → **deadlock**

**✅ روش بهتر با defer:**
```go
mu.Lock()
defer mu.Unlock()  // اجرا میشه وقتی تابع return کنه

statuses[name] = ...
// حتی اگه panic بشه، Unlock اجرا میشه
```

---

### 6️⃣ `ui.go` - نمایش وضعیت

```go
func DisplayStatusLoop() {
    for {
        ClearScreen()
        PrintBanner()

        mu.Lock()
        names := make([]string, 0, len(statuses))
        for name := range statuses {
            names = append(names, name)
        }
        sort.Strings(names)

        // نمایش جدول
        // ...

        mu.Unlock()

        time.Sleep(3 * time.Second)
    }
}
```

#### مفاهیم:

**🔸 Slice Operations:**
```go
names := make([]string, 0, len(statuses))
                     ↑length  ↑capacity
```
- `length` = تعداد فعلی المنت‌ها
- `capacity` = حافظه reserve شده

**🔸 append:**
```go
names = append(names, name)
```
- اضافه کردن به slice
- اگه capacity پر شه، Go خودش resize می‌کنه

**🔸 Sorting:**
```go
sort.Strings(names)
```
- sort کردن slice of strings
- in-place sort (خود slice رو تغییر میده)

**🔸 Printf Formatting:**
```go
fmt.Printf("%-20s", displayName)  // left-aligned, 20 chars
fmt.Printf("%s%-15s%s", color, text, reset)  // با رنگ
```

---

## مفاهیم پیشرفته Go

### 1. Goroutines و Concurrency

```go
// Bad: Race condition
counter := 0
go func() { counter++ }()
go func() { counter++ }()

// Good: با Mutex
var mu sync.Mutex
counter := 0

go func() {
    mu.Lock()
    counter++
    mu.Unlock()
}()
```

### 2. Channels (در این کد استفاده نشده ولی مهمه)

```go
// برای ارتباط بین goroutine‌ها
ch := make(chan string)

go func() {
    ch <- "hello"  // ارسال
}()

msg := <-ch  // دریافت
fmt.Println(msg)
```

### 3. Defer

```go
func readFile() {
    f, err := os.Open("file.txt")
    if err != nil {
        return
    }
    defer f.Close()  // اجرا میشه در آخر تابع

    // حتی اگه panic بشه، Close() صدا زده میشه
}
```

### 4. Error Wrapping (Go 1.13+)

```go
if err != nil {
    return fmt.Errorf("failed to load: %w", err)
}

// بعداً می‌تونی error رو unwrap کنی:
if errors.Is(err, os.ErrNotExist) {
    // ...
}
```

---

## بهترین شیوه‌ها

### ✅ کارهایی که باید انجام بدی:

1. **همیشه error handling:**
```go
// Bad
data, _ := os.ReadFile(file)

// Good
data, err := os.ReadFile(file)
if err != nil {
    return fmt.Errorf("failed to read: %w", err)
}
```

2. **استفاده از defer برای cleanup:**
```go
mu.Lock()
defer mu.Unlock()
```

3. **Context برای cancellation:**
```go
ctx, cancel := context.WithCancel(context.Background())
defer cancel()

go func(ctx context.Context) {
    select {
    case <-ctx.Done():
        return  // cleanup
    }
}(ctx)
```

4. **Validation ورودی:**
```go
if len(name) == 0 {
    return errors.New("name cannot be empty")
}
```

### ❌ کارهایی که نباید انجام بدی:

1. **نادیده گرفتن errors**
2. **فراموش کردن Unlock()**
3. **Race conditions**
4. **Command injection:**

```go
// Bad: کاربر می‌تونه دستور دلخواه inject کنه
cmd := exec.Command("sh", "-c", userInput)

// Better: validate کن
```

---

## نکات امنیتی

### 1. Command Injection

```go
// در service.go:
cmd = exec.Command("bash", "-c", command)
```

**مشکل:** کاربر می‌تونه دستورات خطرناک بده:
```bash
pf add hack "kubectl ...; rm -rf /"
```

**راه‌حل:**
- Validate کردن ورودی
- Whitelist کردن دستورات مجاز
- استفاده از `exec.CommandContext` با timeout

### 2. File Permissions

```go
os.WriteFile(dataFile, data, 0644)
                              ↑ rw-r--r--
```
- `0644` = owner می‌تونه بخونه و بنویسه، بقیه فقط بخونن
- برای اطلاعات حساس: `0600` (فقط owner)

---

## تمرین‌های پیشنهادی

### 🎯 سطح مبتدی:
1. یک دستور `edit` اضافه کن برای ویرایش سرویس
2. پشتیبانی از فیلتر کردن در `list`
3. اضافه کردن timestamp به هر سرویس

### 🎯 سطح متوسط:
1. پشتیبانی از config file (YAML یا TOML)
2. اضافه کردن logging با `log/slog`
3. Graceful shutdown با signal handling

### 🎯 سطح پیشرفته:
1. استفاده از Context برای cancellation
2. پشتیبانی از health check برای هر سرویس
3. Retry logic با exponential backoff
4. پشتیبانی از webhook notifications

---

## منابع مفید

- **Go by Example:** https://gobyexample.com
- **Effective Go:** https://go.dev/doc/effective_go
- **Go Tour:** https://go.dev/tour/
- **Concurrency Patterns:** https://go.dev/blog/pipelines

---

## سوالات متداول

**Q: چرا از map استفاده کردیم نه database؟**
A: برای سادگی. برای production بهتره از SQLite یا یک KV store استفاده کنی.

**Q: چرا همه چی در package main هست؟**
A: برای سادگی. بهتره بزرگ بشی پکیج‌های جدا بسازی:
```
pf/
├── cmd/pf/main.go
├── internal/
│   ├── storage/
│   ├── service/
│   └── ui/
```

**Q: چطور باید test بنویسم؟**
A: با `testing` package:
```go
func TestExtractPorts(t *testing.T) {
    local, remote, ok := extractPorts("kubectl 8080:5432")
    if !ok {
        t.Fatal("expected ok=true")
    }
    if local != "5432" {
        t.Errorf("expected local=5432, got %s", local)
    }
}
```

---

**موفق باشی! 🚀**
