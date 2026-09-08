package treesitter

// Point is a row/column position in a document.
type Point struct {
	Row    uint32
	Column uint32
}

func pointAdd(a, b Point) Point {
	if b.Row > 0 {
		return Point{Row: a.Row + b.Row, Column: b.Column}
	}
	return Point{Row: a.Row, Column: a.Column + b.Column}
}

func pointSub(a, b Point) Point {
	if a.Row > b.Row {
		return Point{Row: a.Row - b.Row, Column: a.Column}
	}
	if a.Column >= b.Column {
		return Point{Row: 0, Column: a.Column - b.Column}
	}
	return Point{}
}

func pointLte(a, b Point) bool {
	return a.Row < b.Row || (a.Row == b.Row && a.Column <= b.Column)
}

func pointLt(a, b Point) bool {
	return a.Row < b.Row || (a.Row == b.Row && a.Column < b.Column)
}

func pointGt(a, b Point) bool {
	return a.Row > b.Row || (a.Row == b.Row && a.Column > b.Column)
}

func pointGte(a, b Point) bool {
	return a.Row > b.Row || (a.Row == b.Row && a.Column >= b.Column)
}

func pointEq(a, b Point) bool { return a.Row == b.Row && a.Column == b.Column }

type length struct {
	Bytes  uint32
	Extent Point
}

var lengthUndefined = length{Bytes: 0, Extent: Point{Row: 0, Column: 1}}

var lengthMax = length{Bytes: ^uint32(0), Extent: Point{Row: ^uint32(0), Column: ^uint32(0)}}

func lengthIsUndefined(l length) bool { return l.Bytes == 0 && l.Extent.Column != 0 }

func lengthMin(a, b length) length {
	if a.Bytes < b.Bytes {
		return a
	}
	return b
}

func lengthAdd(a, b length) length {
	return length{Bytes: a.Bytes + b.Bytes, Extent: pointAdd(a.Extent, b.Extent)}
}

func lengthSub(a, b length) length {
	var bytes uint32
	if a.Bytes >= b.Bytes {
		bytes = a.Bytes - b.Bytes
	}
	return length{Bytes: bytes, Extent: pointSub(a.Extent, b.Extent)}
}

func lengthZero() length { return length{} }

func lengthSaturatingSub(a, b length) length {
	if a.Bytes > b.Bytes {
		return lengthSub(a, b)
	}
	return lengthZero()
}
