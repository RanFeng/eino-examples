/*
 * Copyright 2025 CloudWeGo Authors
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

// Code generated by hertz generator.

package handler

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/RanFeng/ilog"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/sse"

	"github.com/cloudwego/eino-examples/flow/agent/deer-go/biz/consts"
	"github.com/cloudwego/eino-examples/flow/agent/deer-go/biz/eino"
	"github.com/cloudwego/eino-examples/flow/agent/deer-go/biz/infra"
	"github.com/cloudwego/eino-examples/flow/agent/deer-go/biz/model"
	"github.com/cloudwego/eino-examples/flow/agent/deer-go/biz/util"
)

func ChatStreamEino(ctx context.Context, c *app.RequestContext) {
	// 设置响应头（NewStream 会自动设置部分头，但建议显式声明）
	c.SetContentType("text/event-stream; charset=utf-8")
	c.Response.Header.Set("Cache-Control", "no-cache")
	c.Response.Header.Set("Connection", "keep-alive")
	c.Response.Header.Set("Access-Control-Allow-Origin", "*")

	c.SetStatusCode(http.StatusOK)
	// 初始化一个sse writer
	w := sse.NewWriter(c)
	defer w.Close()

	// 请求体校验
	req := new(model.ChatRequest)
	err := c.BindAndValidate(req)
	if err != nil {
		return
	}
	ilog.EventInfo(ctx, "ChatStream_begin", "req", req)

	// 根据前端参数生成Graph State
	genFunc := func(ctx context.Context) *model.State {
		return &model.State{
			MaxPlanIterations:             req.MaxPlanIterations,
			MaxStepNum:                    req.MaxStepNum,
			Messages:                      req.Messages,
			Goto:                          consts.Coordinator,
			EnableBackgroundInvestigation: req.EnableBackgroundInvestigation,
		}
	}

	// Build Graph
	r := eino.Builder[string, string, *model.State](ctx, genFunc)

	// Run Graph
	_, err = r.Stream(ctx, consts.Coordinator,
		compose.WithCheckPointID(req.ThreadID), // 指定Graph的CheckPointID
		// 中断后，获取用户的edit_plan信息
		compose.WithStateModifier(func(ctx context.Context, path compose.NodePath, state any) error {
			s := state.(*model.State)
			s.InterruptFeedback = req.InterruptFeedback
			if req.InterruptFeedback == "edit_plan" {
				s.Messages = append(s.Messages, req.Messages...)
			}
			ilog.EventDebug(ctx, "ChatStream_modf", "path", path.GetPath(), "state", state)
			return nil
		}),
		// 连接LoggerCallback
		compose.WithCallbacks(&infra.LoggerCallback{
			ID:  req.ThreadID,
			SSE: w,
		}),
	)

	// 将interrupt信号传递到前端
	if info, ok := compose.ExtractInterruptInfo(err); ok {
		ilog.EventDebug(ctx, "ChatStream_interrupt", "info", info)
		data := &model.ChatResp{
			ThreadID:     req.ThreadID,
			ID:           "human_feedback:" + util.RandStr(20),
			Role:         "assistant",
			Content:      "检查计划",
			FinishReason: "interrupt",
			Options: []map[string]any{
				{
					"text":  "编辑计划",
					"value": "edit_plan",
				},
				{
					"text":  "开始执行",
					"value": "accepted",
				},
			},
		}
		dB, _ := json.Marshal(data)
		w.WriteEvent("", "interrupt", dB)
	}
	if err != nil {
		ilog.EventError(ctx, err, "ChatStream_error")
	}
}
