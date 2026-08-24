package pagination

import "math"

type Params struct {
	Limit  int32
	Offset int32
}

type Response struct {
	Data       interface{} `json:"data"`
	Limit      int32       `json:"limit"`
	Offset     int32       `json:"offset"`
	Total      int64       `json:"total"`
	TotalPages int         `json:"total_pages"`
}

func NewResponse(data interface{}, p Params, total int64) Response {
	totalPages := 0
	if p.Limit > 0 {
		totalPages = int(math.Ceil(float64(total) / float64(p.Limit)))
	}
	return Response{
		Data:       data,
		Limit:      p.Limit,
		Offset:     p.Offset,
		Total:      total,
		TotalPages: totalPages,
	}
}