package main

func ByteToU32B(b []byte) (i uint32) {
	return 0
}

func U32BE(b []byte) (i uint32) {
	i = uint32(b[0])
	i <<= 8
	i |= uint32(b[1])
	i <<= 8
	i |= uint32(b[2])
	i <<= 8
	i |= uint32(b[3])
	return
}

func IsLittleEndian() bool {
	i16 := int16(0x1234)
	i8 := int8(i16)
	//log.Println(unsafe.Sizeof(i16), unsafe.Sizeof(i8))
	if 0x12 == i8 {
		//log.Println("big endian")
		return false
	}
	//log.Println("little endian")
	return true
}
