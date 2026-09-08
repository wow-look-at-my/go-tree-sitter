package treesitter

const decodeError int32 = -1

var lead3T1Bits = [16]uint8{
	0x20, 0x30, 0x30, 0x30, 0x30, 0x30, 0x30, 0x30,
	0x30, 0x30, 0x30, 0x30, 0x30, 0x10, 0x30, 0x30,
}

var lead4T1Bits = [16]uint8{
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	0x1E, 0x0F, 0x0F, 0x0F, 0x00, 0x00, 0x00, 0x00,
}

// DecodeUTF8 mirrors the reference implementation's byte sequence handling,
// including how far it advances over an ill formed sequence.
func DecodeUTF8(s []byte) (int32, uint32) {
	length := uint32(len(s))
	if length == 0 {
		return decodeError, 0
	}
	i := uint32(0)
	c := int32(s[i])
	i++
	if c < 0x80 {
		return c, i
	}
	var t uint8
	ok := false
	if i != length {
		if c >= 0xE0 {
			if c < 0xF0 {
				c &= 0xF
				t = s[i]
				if lead3T1Bits[c]&(1<<(t>>5)) != 0 {
					t &= 0x3F
					c = (c << 6) | int32(t)
					i++
					if i != length {
						ok = true
					}
				}
			} else {
				c -= 0xF0
				if c <= 4 {
					t = s[i]
					if lead4T1Bits[t>>4]&(1<<uint(c)) != 0 {
						c = (c << 6) | int32(t&0x3F)
						i++
						if i != length {
							t2 := int32(s[i]) - 0x80
							if t2 <= 0x3F && t2 >= 0 {
								c = (c << 6) | t2
								i++
								if i != length {
									ok = true
								}
							}
						}
					}
				}
			}
		} else if c >= 0xC2 {
			c &= 0x1F
			ok = true
		}
	}
	if ok {
		t2 := int32(s[i]) - 0x80
		if t2 <= 0x3F && t2 >= 0 {
			c = (c << 6) | t2
			i++
			return c, i
		}
	}
	return decodeError, i
}

// DecodeUTF16LE decodes one code point from little endian input.
func DecodeUTF16LE(s []byte) (int32, uint32) {
	if len(s) < 2 {
		return decodeError, uint32(len(s))
	}
	c := int32(s[0]) | int32(s[1])<<8
	if c >= 0xD800 && c <= 0xDBFF && len(s) >= 4 {
		c2 := int32(s[2]) | int32(s[3])<<8
		if c2 >= 0xDC00 && c2 <= 0xDFFF {
			return (c << 10) + c2 - 0x35FDC00, 4
		}
	}
	return c, 2
}

// DecodeUTF16BE decodes one code point from big endian input.
func DecodeUTF16BE(s []byte) (int32, uint32) {
	if len(s) < 2 {
		return decodeError, uint32(len(s))
	}
	c := int32(s[0])<<8 | int32(s[1])
	if c >= 0xD800 && c <= 0xDBFF && len(s) >= 4 {
		c2 := int32(s[2])<<8 | int32(s[3])
		if c2 >= 0xDC00 && c2 <= 0xDFFF {
			return (c << 10) + c2 - 0x35FDC00, 4
		}
	}
	return c, 2
}
