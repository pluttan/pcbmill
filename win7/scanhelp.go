package main

// What the operator sees when the scan stops: not one line of the scanner,
// but what happened, why it happens, and what to do, as a card. And the
// corners of the boards touched first, so a wrong zero or a loose wire shows
// in five points, not after an hour.

import (
	"errors"
	"fmt"
	"math"
	"strings"
)

// helpErr: an error with its explanation for the stop card.
type helpErr struct {
	title  string
	what   string
	causes []string
	todo   []string
}

func (h helpErr) Error() string { return h.what }

// lines: the text of the card.
func (h helpErr) lines() []string {
	out := []string{h.what}
	if len(h.causes) > 0 {
		out = append(out, "", "Почему так бывает:")
		for _, c := range h.causes {
			out = append(out, "•  "+c)
		}
	}
	if len(h.todo) > 0 {
		out = append(out, "", "Что сделать:")
		for i, c := range h.todo {
			out = append(out, fmt.Sprintf("%d. %s", i+1, c))
		}
	}
	return out
}

// showErr: the stop card; a helpErr with its causes, any other error as is.
func showErr(err error) {
	var h helpErr
	if errors.As(err, &h) {
		d := &dialog{title: h.title, text: h.lines(), buttons: []string{"Понятно"}, kind: dlgStop}
		d.show()
		return
	}
	alert(err)
}

// scanHelp explains an error of the scan; first: it came at the first
// points, where a wrong zero of Z is the likely cause.
func scanHelp(err error, first bool, saved string) helpErr {
	msg := err.Error()
	h := helpErr{title: "Карта высот не снята", what: msg}
	zero := "Ноль Z в NcStudio не на высоте старта (забыли обнулить или остался ноль от касания на меди): " +
		"обход ведёт Z от этого нуля, и первая же команда вниз уводит фрезу в медь."
	wire := "Провод касания: зажим отошёл, стоит на краске или окисле, медь под ним не связана с точкой касания."
	wifi := "Включён Wi-Fi щупа: он гаснет через 10 минут после загрузки файлов, пока горит, вход касания ловит помехи."
	switch {
	case strings.Contains(msg, "остановлено"):
		h.title, h.causes = "Обход остановлен", nil
	case strings.Contains(msg, "не размыкается"):
		if first {
			h.causes = append(h.causes, zero)
		}
		h.causes = append(h.causes, "Станок не выполнил подъём: команда пропущена.",
			"Провод касания замкнут на фрезу или на раму станка.")
	case strings.Contains(msg, "медь не найдена"):
		if first {
			h.causes = append(h.causes, "Фреза стояла выше 3 мм над медью: первая точка ищет медь только на 3 мм вниз.")
		}
		h.causes = append(h.causes, wire, "Край заготовки или вырезанное место под точкой.")
	case strings.Contains(msg, "контакт до переезда"):
		h.causes = append(h.causes, "Провод касания замкнут на фрезу или на раму станка.", wifi,
			"Фреза уже стоит на меди: её не подняло.")
	case strings.Contains(msg, "повторное касание"):
		h.causes = append(h.causes, wifi, wire)
	case strings.Contains(msg, "во время переезда"), strings.Contains(msg, "выше всех прежних"):
		if first {
			h.causes = append(h.causes, zero)
		}
		h.causes = append(h.causes, "Станок пропустил команду подъёма.", "Заготовка выше, чем ожидалось: отошла от стола.")
	default:
		if first {
			h.causes = append(h.causes, zero)
		}
		h.causes = append(h.causes, wire, wifi)
	}
	if h.title != "Обход остановлен" {
		h.todo = append(h.todo, "Поднимите фрезу и проверьте провод: Сервис / Проверить провод.")
		if first {
			h.todo = append(h.todo, "Поставьте фрезу на 1-2 мм над первой точкой ♦ и обнулите Z в NcStudio на этой высоте.")
		}
		h.todo = append(h.todo, "Запустите карту высот заново.")
	}
	if saved != "" {
		h.todo = append(h.todo, "Снятые точки сохранены в "+saved+".")
	}
	return h
}

// cornersFirst: the first point (the zero) stays first, then the point
// nearest each corner of the rest, then the rest in its order.
func cornersFirst(pts [][2]float64) [][2]float64 {
	if len(pts) < 6 {
		return pts
	}
	x0, x1, y0, y1 := math.Inf(1), math.Inf(-1), math.Inf(1), math.Inf(-1)
	for _, p := range pts[1:] {
		x0, x1 = math.Min(x0, p[0]), math.Max(x1, p[0])
		y0, y1 = math.Min(y0, p[1]), math.Max(y1, p[1])
	}
	taken := map[int]bool{0: true}
	out := [][2]float64{pts[0]}
	for _, c := range [][2]float64{{x0, y0}, {x1, y0}, {x1, y1}, {x0, y1}} {
		best, bd := -1, math.Inf(1)
		for i, p := range pts {
			if d := math.Hypot(p[0]-c[0], p[1]-c[1]); !taken[i] && d < bd {
				best, bd = i, d
			}
		}
		taken[best] = true
		out = append(out, pts[best])
	}
	for i, p := range pts {
		if !taken[i] {
			out = append(out, p)
		}
	}
	return out
}

// cornerReport: the corners against the first point, and whether they look
// like copper under a tool zeroed right.
func cornerReport(rows []Point) (lines []string, odd bool) {
	lines = []string{"Высота меди в углах от первой точки ♦:"}
	lo, hi := math.Inf(1), math.Inf(-1)
	for _, r := range rows[1:] {
		d := r.Z - rows[0].Z
		lo, hi = math.Min(lo, d), math.Max(hi, d)
		lines = append(lines, fmt.Sprintf("X %5.1f  Y %5.1f:   %+.3f мм", r.X, r.Y, d))
	}
	spread := math.Max(hi, 0) - math.Min(lo, 0)
	lines = append(lines, "", fmt.Sprintf("Перепад %.3f мм.", spread))
	if spread > 0.5 {
		odd = true
		lines = append(lines, "▲ Больше 0.5 мм: так заготовка не лежит. Проверьте ноль Z, провод и крепление заготовки.")
	} else {
		lines = append(lines, "√ Заготовка лежит ровно, касание работает.")
	}
	return lines, odd
}
