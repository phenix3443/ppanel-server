package httpx

type ResponseSuccessBean struct {
	Code uint32      `json:"code"`
	Msg  string      `json:"msg"`
	Data interface{} `json:"data,omitempty"`
}                      // @name result.ResponseSuccessBean
type NullJson struct{} // @name result.NullJson

func Success(data interface{}) *ResponseSuccessBean {
	return &ResponseSuccessBean{200, "success", data}
}

type ResponseErrorBean struct {
	Code uint32 `json:"code"`
	Msg  string `json:"msg"`
} // @name result.ResponseErrorBean

func Error(errCode uint32, errMsg string) *ResponseErrorBean {
	return &ResponseErrorBean{errCode, errMsg}
}
