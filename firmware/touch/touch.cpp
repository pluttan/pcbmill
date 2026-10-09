// Touch sensor for height mapping: copper clip on TOUCH_PIN, MCU GND on the bit.
// The bit touching copper pulls the pin low.
//
// Serial 115200:
//   "T1" / "T0" sent the moment contact appears / disappears (debounced)
//   "?" -> "S1" / "S0"  current state
//   "P" -> "PCBTOUCH 1" identification
// OUT_PIN mirrors contact (LOW = touching) so it can also feed a machine input.
//
// ESP32 also carries files to the machine PC, which has no network: a Wi-Fi
// point "pcbscan" (password pcbmill1; off until BOOT or "A") with a page at http://192.168.4.1 takes
// files (the page gzips them) into the flash; pcbscan takes them over the port:
//   "L"      -> "E <n> <bytes> <md5> <name>" per file, then "END"
//   "R" + c  -> read file n = c - '0'; "N" -> its next CHUNK bytes, raw
//   "F"      -> read the newest file: "FILE <bytes> <md5> <name>" or "NOFILE"
//   "A" / "a" -> the Wi-Fi point on (for 10 idle minutes) / off; BOOT is "A"
//   "W"      -> "WIFI up|down <ip> clients <n> power <q>": is the point there
//   "S"      -> a PowerShell 2.0 script that takes the newest file over $p;
//               the first time, on the PC, one typed line runs it:
//   $p=New-Object IO.Ports.SerialPort COM5,115200;$p.Open();$p.DiscardInBuffer();$p.Write('S');sleep 1;iex $p.ReadExisting()
// While a file goes out, T1/T0 wait so they do not land in the data.
//
// pluttan

#include <Arduino.h>

#ifndef TOUCH_PIN
#define TOUCH_PIN 2
#endif
#ifndef OUT_PIN
#define OUT_PIN 3
#endif
#ifndef LED_PIN
#define LED_PIN LED_BUILTIN
#endif

// contact must read the same this many times in a row (~5 us each) before it counts:
// long clip wires pick up spikes from steppers and the VFD
const uint8_t STABLE = 8;

bool state = false;
bool sent = false; // the state last reported
uint8_t same = 0;

#ifdef ESP32
#include <WiFi.h>
#include <WebServer.h>
#include <MD5Builder.h>

// ==========================
// ===  The file store    ===
// ==========================

// app1 and spiffs of the default table, unused: a table sector, then the files
// one after another, each from a sector boundary. Written once, cleared whole.
const uint32_t STORE = 0x150000, STORE_END = 0x3F0000, SECTOR = 4096;
const uint32_t DATA = STORE + SECTOR;
const int SLOTS = 63;
const size_t CHUNK = 4096;

struct Entry {        // 64 bytes; erased flash reads 0xFF: a free slot
  uint32_t used;      // 1
  uint32_t off, size; // from STORE
  uint8_t md5[16];
  char name[36];
};

Entry table[SLOTS];
uint32_t buf[CHUNK / 4];

void loadTable() { ESP.flashRead(STORE + 64, (uint32_t *)table, sizeof(table)); }

bool isUsed(int i) { return table[i].used == 1; }

uint32_t freeAt() { // the first byte after the last file, on a sector boundary
  uint32_t at = DATA;
  for (int i = 0; i < SLOTS; i++)
    if (isUsed(i)) {
      uint32_t end = STORE + table[i].off + table[i].size;
      end = (end + SECTOR - 1) / SECTOR * SECTOR;
      if (end > at) at = end;
    }
  return at;
}

void clearStore() {
  for (uint32_t a = STORE; a < freeAt(); a += SECTOR) ESP.flashEraseSector(a / SECTOR);
  ESP.flashEraseSector(STORE / SECTOR);
  loadTable();
}

String md5hex(const uint8_t *m) {
  char s[33];
  for (int i = 0; i < 16; i++) sprintf(s + 2 * i, "%02x", m[i]);
  return String(s);
}

// ==========================
// ===  Upload over Wi-Fi ===
// ==========================

WebServer web(80);
bool apUp = false; // softAP started
unsigned long apLast; // the last sign of use: on, a client, an upload

// The radio is off unless asked for (the BOOT button, "A" from pcbscan): next
// to the antenna the touch input read contact all the time while it was on.
const int BOOT_PIN = 9;
const unsigned long AP_IDLE = 10 * 60 * 1000UL;

void apOn() {
  apLast = millis();
  if (apUp) return;
  // the SuperMini's regulator cannot feed full power: the point drops or
  // does not show; set lower after softAP, before it the setting is lost
  WiFi.mode(WIFI_AP);
  apUp = WiFi.softAP("pcbscan", "pcbmill1", 6);
  WiFi.setTxPower(WIFI_POWER_8_5dBm);
  web.begin();
}

void apOff() {
  if (!apUp) return;
  web.stop();
  WiFi.softAPdisconnect(true);
  WiFi.mode(WIFI_OFF);
  apUp = false;
}
uint32_t upAt, upStart, upFill;
bool upFail;
String upName, upMsg;
MD5Builder upMd5;
uint8_t *upBuf = (uint8_t *)buf;

void upFlush() {
  if (upAt + SECTOR > STORE_END) {
    upFail = true;
    return;
  }
  memset(upBuf + upFill, 0xFF, SECTOR - upFill);
  ESP.flashEraseSector(upAt / SECTOR);
  ESP.flashWrite(upAt, buf, SECTOR);
  upAt += SECTOR;
  upFill = 0;
}

void onUpload() {
  HTTPUpload &u = web.upload();
  if (u.status == UPLOAD_FILE_START) {
    upName = u.filename.substring(u.filename.lastIndexOf('/') + 1);
    upStart = upAt = freeAt();
    upFill = 0;
    upFail = false;
    upMd5.begin();
  } else if (u.status == UPLOAD_FILE_WRITE) {
    upMd5.add(u.buf, u.currentSize);
    for (size_t k = 0; k < u.currentSize && !upFail;) {
      size_t n = min((size_t)(SECTOR - upFill), u.currentSize - k);
      memcpy(upBuf + upFill, u.buf + k, n);
      upFill += n;
      k += n;
      if (upFill == SECTOR) upFlush();
    }
  } else if (u.status == UPLOAD_FILE_END) {
    if (upFill) upFlush();
    int slot = -1;
    for (int i = 0; i < SLOTS && slot < 0; i++)
      if (!isUsed(i)) slot = i;
    if (upFail || slot < 0 || upName.length() == 0 || upName.length() > 35) {
      upMsg = "Не поместилось: " + upName + ". Очистите память.";
      return;
    }
    Entry e;
    memset(&e, 0, sizeof(e));
    e.used = 1;
    e.off = upStart - STORE;
    e.size = u.totalSize;
    upMd5.calculate();
    upMd5.getBytes(e.md5);
    strncpy(e.name, upName.c_str(), sizeof(e.name) - 1);
    ESP.flashWrite(STORE + 64 + slot * sizeof(Entry), (uint32_t *)&e, sizeof(e));
    loadTable();
    upMsg = "Принят " + upName;
  }
}

const char PAGE_HEAD[] = R"HTML(<!doctype html><meta charset=utf-8><meta name=viewport content="width=device-width">
<title>pcbscan</title><style>body{font:16px sans-serif;background:#111;color:#ddd;margin:20px;max-width:640px}
td{padding:4px 12px 4px 0}button,input{font-size:16px;margin:8px 0}</style>
<h2>Файлы для станка</h2>)HTML";

const char PAGE_TAIL[] = R"HTML(<p><input type=file id=f multiple><br><button onclick=send()>Загрузить</button> <span id=s></span>
<form method=post action=/clear onsubmit="return confirm('Удалить все файлы?')"><button>Очистить память</button></form>
<p>pcbscan на компе станка заберёт новые файлы сам: схемы лягут рядом с ним, pcbscan.exe обновит программу.
<script>
async function send(){
 const s=document.getElementById('s');
 for(const f of document.getElementById('f').files){
  s.textContent='сжимаю '+f.name;
  let b=f,n=f.name;
  if(window.CompressionStream&&!n.endsWith('.gz')){b=await new Response(f.stream().pipeThrough(new CompressionStream('gzip'))).blob();n+='.gz'}
  s.textContent='загружаю '+n+', '+Math.round(b.size/1024)+' КБ';
  const d=new FormData();d.append('f',b,n);
  const r=await fetch('/upload',{method:'POST',body:d});
  s.textContent=await r.text();if(!r.ok)return}
 location.reload()}
</script>)HTML";

void onPage() {
  String h = PAGE_HEAD;
  h += "<table>";
  int n = 0;
  for (int i = 0; i < SLOTS; i++)
    if (isUsed(i)) {
      h += "<tr><td>" + String(table[i].name) + "<td>" + String(table[i].size / 1024) + " КБ";
      n++;
    }
  h += "</table>";
  if (!n) h += "<p>Файлов нет.";
  h += "<p>Свободно " + String((STORE_END - freeAt()) / 1024) + " КБ.";
  h += PAGE_TAIL;
  web.send(200, "text/html; charset=utf-8", h);
}

// ==========================
// ===  Files to the PC   ===
// ==========================

uint32_t rdOff, rdLen, rdAt;
unsigned long rdLast;

bool reading() { return rdAt < rdLen && millis() - rdLast < 3000; }

void readStart(int i) {
  rdOff = rdLen = rdAt = 0;
  if (i < 0 || i >= SLOTS || !isUsed(i)) return;
  rdOff = STORE + table[i].off;
  rdLen = table[i].size;
  rdLast = millis();
}

void readNext() {
  if (rdAt >= rdLen) return;
  size_t n = rdLen - rdAt;
  if (n > CHUNK) n = CHUNK;
  ESP.flashRead(rdOff + rdAt, buf, (n + 3) & ~3u);
  Serial.write((const uint8_t *)buf, n);
  Serial.flush();
  rdAt += n;
  rdLast = millis();
}

void list() {
  for (int i = 0; i < SLOTS; i++)
    if (isUsed(i))
      Serial.printf("E %d %u %s %s\n", i, (unsigned)table[i].size, md5hex(table[i].md5).c_str(), table[i].name);
  Serial.print("END\n");
}

void newest() {
  int last = -1;
  for (int i = 0; i < SLOTS; i++)
    if (isUsed(i)) last = i;
  if (last < 0) {
    Serial.print("NOFILE\n");
    return;
  }
  readStart(last);
  Serial.printf("FILE %u %s %s\n", (unsigned)table[last].size, md5hex(table[last].md5).c_str(), table[last].name);
}

// the first time: takes the newest file into the folder PowerShell is in, ungzipped
const char RECV[] = R"PS($p.ReadTimeout=5000;$p.DiscardInBuffer();$p.Write('F')
do{$h=$p.ReadLine()}until($h -like 'FILE*' -or $h -like 'NOFILE*')
if($h -like 'NOFILE*'){'no file in the probe';$p.Close()}else{
$a=$h.Split(' ');$n=[int]$a[1];$b=New-Object byte[] 4096;$g=0;$ms=New-Object IO.MemoryStream
while($g -lt $n){$p.Write('N');$k=[Math]::Min(4096,$n-$g);$r=0
while($r -lt $k){$r+=$p.Read($b,$r,$k-$r)};$ms.Write($b,0,$k);$g+=$k
if($g % 262144 -lt 4096){Write-Host -NoNewline '.'}}
$p.Close();''
$m=-join([Security.Cryptography.MD5]::Create().ComputeHash($ms.ToArray())|%{$_.ToString('x2')})
if($m -ne $a[2]){'md5 differs, run again'}else{
$o=$a[3..($a.Length-1)] -join ' ';$ms.Position=0;$src=$ms
if($o.EndsWith('.gz')){$o=$o.Substring(0,$o.Length-3);$src=New-Object IO.Compression.GZipStream($ms,[IO.Compression.CompressionMode]::Decompress)}
$f=[IO.File]::Create("$pwd\$o");do{$r=$src.Read($b,0,4096);$f.Write($b,0,$r)}while($r -gt 0);$f.Close();"$pwd\$o"}}
)PS";
#endif

void report() {
  sent = state;
  Serial.print(state ? "T1\n" : "T0\n");
}

void setup() {
  pinMode(TOUCH_PIN, INPUT_PULLUP);
  pinMode(OUT_PIN, OUTPUT);
  pinMode(LED_PIN, OUTPUT);
  digitalWrite(OUT_PIN, HIGH);
  Serial.begin(115200);
  state = digitalRead(TOUCH_PIN) == LOW;
  Serial.print("PCBTOUCH 1\n");
  report();
#ifdef ESP32
  loadTable();
  pinMode(BOOT_PIN, INPUT_PULLUP);
  web.on("/", HTTP_GET, onPage);
  web.on("/upload", HTTP_POST, [] { web.send(upMsg.startsWith("Принят") ? 200 : 507, "text/plain; charset=utf-8", upMsg); },
         onUpload);
  web.on("/clear", HTTP_POST, [] {
    clearStore();
    web.sendHeader("Location", "/");
    web.send(303);
  });
#endif
}

#ifdef ESP32
int pendingR = 0; // "R" came, the number is the next byte
#endif

void loop() {
  bool now = digitalRead(TOUCH_PIN) == LOW;
  if (now != state) {
    if (++same >= STABLE) {
      state = now;
      same = 0;
      digitalWrite(OUT_PIN, state ? LOW : HIGH);
      digitalWrite(LED_PIN, state ? HIGH : LOW);
    }
  } else {
    same = 0;
  }
#ifdef ESP32
  if (state != sent && !reading()) report();
#else
  if (state != sent) report();
#endif
  while (Serial.available()) {
    char c = Serial.read();
#ifdef ESP32
    if (pendingR) {
      pendingR = 0;
      readStart(c - '0');
      continue;
    }
#endif
    if (c == '?') Serial.print(state ? "S1\n" : "S0\n");
    else if (c == 'P') Serial.print("PCBTOUCH 1\n");
#ifdef ESP32
    else if (c == 'L') list();
    else if (c == 'R') pendingR = 1;
    else if (c == 'N') readNext();
    else if (c == 'F') newest();
    else if (c == 'S') Serial.print(RECV);
    else if (c == 'A') apOn();
    else if (c == 'a') apOff();
    else if (c == 'W')
      Serial.printf("WIFI %s %s clients %d power %d\n", apUp ? "up" : "down", WiFi.softAPIP().toString().c_str(),
                    WiFi.softAPgetStationNum(), (int)WiFi.getTxPower());
#endif
  }
#ifdef ESP32
  if (digitalRead(BOOT_PIN) == LOW) apOn();
  if (apUp) {
    web.handleClient();
    if (WiFi.softAPgetStationNum() > 0) apLast = millis();
    if (millis() - apLast > AP_IDLE) apOff();
  }
#endif
}
