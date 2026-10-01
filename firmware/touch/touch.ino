// Touch sensor for height mapping: copper clip on TOUCH_PIN, MCU GND on the bit.
// The bit touching copper pulls the pin low.
//
// Serial 115200:
//   "T1" / "T0" sent the moment contact appears / disappears (debounced)
//   "?" -> "S1" / "S0"  current state
//   "P" -> "PCBTOUCH 1" identification
// OUT_PIN mirrors contact (LOW = touching) so it can also feed a machine input.
//
// pluttan

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
uint8_t same = 0;

void report() {
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
}

void loop() {
  bool now = digitalRead(TOUCH_PIN) == LOW;
  if (now != state) {
    if (++same >= STABLE) {
      state = now;
      same = 0;
      digitalWrite(OUT_PIN, state ? LOW : HIGH);
      digitalWrite(LED_PIN, state ? HIGH : LOW);
      report();
    }
  } else {
    same = 0;
  }
  while (Serial.available()) {
    char c = Serial.read();
    if (c == '?') Serial.print(state ? "S1\n" : "S0\n");
    else if (c == 'P') Serial.print("PCBTOUCH 1\n");
  }
}
