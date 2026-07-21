package mysql

import (
	"database/sql/driver"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"strings"
	"time"
)

func parseDateTime(str string, loc *time.Location) (t time.Time, err error) {
	var tUTC time.Time
	switch len(str) {
	case 10: // YYYY-MM-DD
		tUTC, err = time.ParseInLocation("2006-01-02", str, time.UTC)
	case 19: // YYYY-MM-DD HH:MM:SS
		tUTC, err = time.ParseInLocation("2006-01-02 15:04:05", str, time.UTC)
	default:
		if len(str) > 19 && str[19] == '.' {
			tUTC, err = time.ParseInLocation("2006-01-02 15:04:05.999999999", str, time.UTC)
		} else if len(str) < 10 {
			err = errors.New("invalid time")
		} else {
			tUTC, err = time.ParseInLocation("2006-01-02 15:04:05.999999999"[:len(str)], str, time.UTC)
		}
	}
	if err != nil {
		return
	}
	return parseDateTimeWithLoc(
		tUTC.Year(),
		int(tUTC.Month()),
		tUTC.Day(),
		tUTC.Hour(),
		tUTC.Minute(),
		tUTC.Second(),
		tUTC.Nanosecond(),
		loc,
	)
}

func parseBinaryDateTime(num uint64, data []byte, loc *time.Location) (time.Time, error) {
	switch num {
	case 0:
		return time.Time{}, nil
	case 4:
		return parseDateTimeWithLoc(
			int(binary.LittleEndian.Uint16(data[:2])),
			int(data[2]),
			int(data[3]),
			0, 0, 0, 0,
			loc,
		)
	case 7:
		return parseDateTimeWithLoc(
			int(binary.LittleEndian.Uint16(data[:2])),
			int(data[2]),
			int(data[3]),
			int(data[4]),
			int(data[5]),
			int(data[6]),
			0,
			loc,
		)
	case 11:
		return parseDateTimeWithLoc(
			int(binary.LittleEndian.Uint16(data[:2])),
			int(data[2]),
			int(data[3]),
			int(data[4]),
			int(data[5]),
			int(data[6]),
			int(binary.LittleEndian.Uint32(data[7:11]))*1000,
			loc,
		)
	}
	return time.Time{}, errors.New("invalid binary datetime length")
}

func parseDateTimeWithLoc(year, month, day, hour, min, sec, nsec int, loc *time.Location) (time.Time, error) {
	if loc == nil {
		loc = time.UTC
	}
	if year == 0 && month == 0 && day == 0 {
		return time.Date(0, 0, 0, hour, min, sec, nsec, loc), nil
	}
	t := time.Date(year, month, day, hour, min, sec, nsec, loc)
	if t.Year() == year && t.Month() == time.Month(month) && t.Day() == day &&
		t.Hour() == hour && t.Minute() == min && t.Second() == sec && t.Nanosecond() == nsec {
		return t, nil
	}
	name, offset := t.Zone()
	return time.Date(year, month, day, hour, min, sec, nsec, time.FixedZone(name, offset)), nil
}

func readLengthEncodedInteger(data []byte) (num uint64, isNull bool, n int) {
	if len(data) == 0 {
		return 0, true, 0
	}
	switch data[0] {
	case 0xfb:
		return 0, true, 1
	case 0xfc:
		if len(data) < 3 {
			return 0, true, 1
		}
		return uint64(binary.LittleEndian.Uint16(data[1:3])), false, 3
	case 0xfd:
		if len(data) < 4 {
			return 0, true, 1
		}
		return uint64(data[1]) | uint64(data[2])<<8 | uint64(data[3])<<16, false, 4
	case 0xfe:
		if len(data) < 9 {
			return 0, true, 1
		}
		return binary.LittleEndian.Uint64(data[1:9]), false, 9
	}
	return uint64(data[0]), false, 1
}

func escapeStringBackslash(dst, src []byte) []byte {
	for _, b := range src {
		switch b {
		case 0:
			dst = append(dst, '\\', '0')
		case '\n':
			dst = append(dst, '\\', 'n')
		case '\r':
			dst = append(dst, '\\', 'r')
		case '\\':
			dst = append(dst, '\\', '\\')
		case '\'':
			dst = append(dst, '\\', '\'')
		case '"':
			dst = append(dst, '\\', '"')
		case 0x1a:
			dst = append(dst, '\\', 'Z')
		default:
			dst = append(dst, b)
		}
	}
	return dst
}

func escapeBytesBackslash(dst, src []byte) []byte {
	return escapeStringBackslash(dst, src)
}

func escapeStringQuotes(dst, src []byte) []byte {
	for _, b := range src {
		if b == '\'' {
			dst = append(dst, '\'', '\'')
		} else {
			dst = append(dst, b)
		}
	}
	return dst
}

func escapeBytesQuotes(dst, src []byte) []byte {
	return escapeStringQuotes(dst, src)
}
