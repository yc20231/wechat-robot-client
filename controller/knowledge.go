package controller

import (
	"context"
	"errors"
	"log"
	"wechat-robot-client/dto"
	"wechat-robot-client/pkg/appx"
	"wechat-robot-client/pkg/qdrantx"
	"wechat-robot-client/service"
	"wechat-robot-client/utils"
	"wechat-robot-client/vars"

	"github.com/gin-gonic/gin"
)

type Knowledge struct{}

func NewKnowledgeController() *Knowledge {
	return &Knowledge{}
}

// ensureKnowledgeService 校验文本知识库服务已初始化。
// AI 配置缺失时 startup.InitRAGService 会把 vars.KnowledgeService 置为 nil 且不中断启动，
// 此时对该接口变量调用方法会 panic，因此所有入口都必须先检查。
func ensureKnowledgeService(resp *appx.Response) bool {
	if vars.KnowledgeService == nil {
		resp.ToErrorResponse(errors.New("RAG 服务未初始化，请先完成 AI 配置"))
		return false
	}
	return true
}

// AddDocument 添加知识库文档
func (k *Knowledge) AddDocument(c *gin.Context) {
	resp := appx.NewResponse(c)
	if !ensureKnowledgeService(resp) {
		return
	}
	var req dto.AddKnowledgeDocumentRequest
	if ok, _ := appx.BindAndValid(c, &req); !ok {
		resp.ToErrorResponse(errors.New("参数错误"))
		return
	}
	if req.Source == "" {
		req.Source = "manual"
	}
	err := vars.KnowledgeService.AddDocument(c.Request.Context(), req.Title, req.Content, req.Source, req.Category)
	if err != nil {
		resp.ToErrorResponse(err)
		return
	}
	resp.ToResponse(nil)
}

// UpdateDocument 更新知识库文档
func (k *Knowledge) UpdateDocument(c *gin.Context) {
	resp := appx.NewResponse(c)
	if !ensureKnowledgeService(resp) {
		return
	}
	var req dto.UpdateKnowledgeDocumentRequest
	if ok, _ := appx.BindAndValid(c, &req); !ok {
		resp.ToErrorResponse(errors.New("参数错误"))
		return
	}
	if req.Source == "" {
		req.Source = "manual"
	}
	err := vars.KnowledgeService.UpdateDocument(c.Request.Context(), req.ID, req.Title, req.Content, req.Source)
	if err != nil {
		resp.ToErrorResponse(err)
		return
	}
	resp.ToResponse(nil)
}

// DeleteDocument 删除知识库文档
func (k *Knowledge) DeleteDocument(c *gin.Context) {
	resp := appx.NewResponse(c)
	if !ensureKnowledgeService(resp) {
		return
	}
	var req dto.DeleteKnowledgeRequest
	if ok, _ := appx.BindAndValid(c, &req); !ok {
		resp.ToErrorResponse(errors.New("参数错误"))
		return
	}
	ctx := c.Request.Context()
	var err error
	if req.ID > 0 {
		err = vars.KnowledgeService.DeleteDocumentByID(ctx, req.ID)
	} else if req.Title != "" {
		err = vars.KnowledgeService.DeleteDocument(ctx, req.Title)
	} else {
		resp.ToErrorResponse(errors.New("请提供 id 或 title"))
		return
	}
	if err != nil {
		resp.ToErrorResponse(err)
		return
	}
	resp.ToResponse(nil)
}

// ListDocuments 获取知识库文档列表
func (k *Knowledge) ListDocuments(c *gin.Context) {
	resp := appx.NewResponse(c)
	if !ensureKnowledgeService(resp) {
		return
	}
	var req dto.ListKnowledgeRequest
	if ok, _ := appx.BindAndValid(c, &req); !ok {
		resp.ToErrorResponse(errors.New("参数错误"))
		return
	}
	pager := appx.InitPager(c)
	docs, total, err := vars.KnowledgeService.ListDocuments(c.Request.Context(), req.Category, pager)
	if err != nil {
		resp.ToErrorResponse(err)
		return
	}
	resp.ToResponseList(docs, total)
}

func (k *Knowledge) EnableDocument(c *gin.Context) {
	resp := appx.NewResponse(c)
	if !ensureKnowledgeService(resp) {
		return
	}
	var req struct {
		ID int64 `json:"id" binding:"required"`
	}
	if ok, _ := appx.BindAndValid(c, &req); !ok {
		resp.ToErrorResponse(errors.New("参数错误"))
		return
	}
	err := vars.KnowledgeService.EnableDocument(c.Request.Context(), req.ID)
	if err != nil {
		resp.ToErrorResponse(err)
		return
	}
	resp.ToResponse(nil)
}

func (k *Knowledge) DisableDocument(c *gin.Context) {
	resp := appx.NewResponse(c)
	if !ensureKnowledgeService(resp) {
		return
	}
	var req struct {
		ID int64 `json:"id" binding:"required"`
	}
	if ok, _ := appx.BindAndValid(c, &req); !ok {
		resp.ToErrorResponse(errors.New("参数错误"))
		return
	}
	err := vars.KnowledgeService.DisableDocument(c.Request.Context(), req.ID)
	if err != nil {
		resp.ToErrorResponse(err)
		return
	}
	resp.ToResponse(nil)
}

// SearchKnowledge 搜索知识库
func (k *Knowledge) SearchKnowledge(c *gin.Context) {
	resp := appx.NewResponse(c)
	if !ensureKnowledgeService(resp) {
		return
	}
	var req dto.SearchKnowledgeRequest
	if ok, _ := appx.BindAndValid(c, &req); !ok {
		resp.ToErrorResponse(errors.New("参数错误"))
		return
	}
	if req.Limit <= 0 {
		req.Limit = 5
	}
	results, err := vars.KnowledgeService.SearchKnowledge(c.Request.Context(), req.Query, req.Category, req.Limit)
	if err != nil {
		resp.ToErrorResponse(err)
		return
	}
	resp.ToResponse(results)
}

// ReindexAll 重建知识库索引
func (k *Knowledge) ReindexAll(c *gin.Context) {
	resp := appx.NewResponse(c)
	if !ensureKnowledgeService(resp) {
		return
	}
	// 后台任务不能复用请求 context：请求响应返回时它会立即被取消，
	// 导致重建在第一次向量写入时就 context canceled。
	go func() {
		log.Printf("[Knowledge] 开始重建知识库索引")
		if err := vars.KnowledgeService.ReindexAll(context.Background()); err != nil {
			log.Printf("[Knowledge] 重建知识库索引失败: %v", err)
			return
		}
		log.Printf("[Knowledge] 重建知识库索引完成")
	}()
	resp.ToResponse("reindex started")
}

// --- 图片知识库接口 ---

// AddImageDocument 添加图片知识库文档
func (k *Knowledge) AddImageDocument(c *gin.Context) {
	resp := appx.NewResponse(c)
	var req dto.AddImageKnowledgeRequest
	if ok, _ := appx.BindAndValid(c, &req); !ok {
		resp.ToErrorResponse(errors.New("参数错误"))
		return
	}
	if vars.ImageKnowledgeService == nil {
		resp.ToErrorResponse(errors.New("图片知识库服务未初始化，请先配置图片嵌入模型"))
		return
	}
	err := vars.ImageKnowledgeService.AddImageDocument(c.Request.Context(), req.Title, req.Description, req.ImageURL, req.Category)
	if err != nil {
		resp.ToErrorResponse(err)
		return
	}
	resp.ToResponse(nil)
}

// DeleteImageDocument 删除图片知识库文档
func (k *Knowledge) DeleteImageDocument(c *gin.Context) {
	resp := appx.NewResponse(c)
	var req dto.DeleteImageKnowledgeRequest
	if ok, _ := appx.BindAndValid(c, &req); !ok {
		resp.ToErrorResponse(errors.New("参数错误"))
		return
	}
	if vars.ImageKnowledgeService == nil {
		resp.ToErrorResponse(errors.New("图片知识库服务未初始化"))
		return
	}
	ctx := c.Request.Context()
	var err error
	if req.ID > 0 {
		err = vars.ImageKnowledgeService.DeleteImageDocumentByID(ctx, req.ID)
	} else if req.Title != "" {
		err = vars.ImageKnowledgeService.DeleteImageDocument(ctx, req.Title)
	} else {
		resp.ToErrorResponse(errors.New("请提供 id 或 title"))
		return
	}
	if err != nil {
		resp.ToErrorResponse(err)
		return
	}
	resp.ToResponse(nil)
}

// ListImageDocuments 获取图片知识库文档列表
func (k *Knowledge) ListImageDocuments(c *gin.Context) {
	resp := appx.NewResponse(c)
	var req dto.ListImageKnowledgeRequest
	if ok, _ := appx.BindAndValid(c, &req); !ok {
		resp.ToErrorResponse(errors.New("参数错误"))
		return
	}
	if vars.ImageKnowledgeService == nil {
		resp.ToErrorResponse(errors.New("图片知识库服务未初始化"))
		return
	}
	pager := appx.InitPager(c)
	docs, total, err := vars.ImageKnowledgeService.ListImageDocuments(c.Request.Context(), req.Category, pager)
	if err != nil {
		resp.ToErrorResponse(err)
		return
	}
	resp.ToResponseList(docs, total)
}

// SearchImageByText 以文搜图
func (k *Knowledge) SearchImageByText(c *gin.Context) {
	resp := appx.NewResponse(c)
	var req dto.SearchImageKnowledgeByTextRequest
	if ok, _ := appx.BindAndValid(c, &req); !ok {
		resp.ToErrorResponse(errors.New("参数错误"))
		return
	}
	if vars.ImageKnowledgeService == nil {
		resp.ToErrorResponse(errors.New("图片知识库服务未初始化"))
		return
	}
	if req.Limit <= 0 {
		req.Limit = 5
	}
	results, err := vars.ImageKnowledgeService.SearchByText(c.Request.Context(), req.Query, req.Category, req.Limit)
	if err != nil {
		resp.ToErrorResponse(err)
		return
	}
	resp.ToResponse(results)
}

// SearchImageByImage 以图搜图
func (k *Knowledge) SearchImageByImage(c *gin.Context) {
	resp := appx.NewResponse(c)
	var req dto.SearchImageKnowledgeByImageRequest
	if ok, _ := appx.BindAndValid(c, &req); !ok {
		resp.ToErrorResponse(errors.New("参数错误"))
		return
	}
	if vars.ImageKnowledgeService == nil {
		resp.ToErrorResponse(errors.New("图片知识库服务未初始化"))
		return
	}
	if req.Limit <= 0 {
		req.Limit = 5
	}
	results, err := vars.ImageKnowledgeService.SearchByImage(c.Request.Context(), req.ImageURL, req.Category, req.Limit)
	if err != nil {
		resp.ToErrorResponse(err)
		return
	}
	resp.ToResponse(results)
}

// ReindexAllImages 重建图片知识库索引
func (k *Knowledge) ReindexAllImages(c *gin.Context) {
	resp := appx.NewResponse(c)
	if vars.ImageKnowledgeService == nil {
		resp.ToErrorResponse(errors.New("图片知识库服务未初始化"))
		return
	}
	// 同 ReindexAll：后台任务使用独立 context，避免被请求结束连带取消。
	go func() {
		log.Printf("[ImageKnowledge] 开始重建图片知识库索引")
		if err := vars.ImageKnowledgeService.ReindexAll(context.Background()); err != nil {
			log.Printf("[ImageKnowledge] 重建图片知识库索引失败: %v", err)
			return
		}
		log.Printf("[ImageKnowledge] 重建图片知识库索引完成")
	}()
	resp.ToResponse("image reindex started")
}

// ReindexAllVectors 全量重建向量索引
func (k *Knowledge) ReindexAllVectors(c *gin.Context) {
	resp := appx.NewResponse(c)

	if vars.QdrantClient == nil {
		resp.ToErrorResponse(errors.New("Qdrant 未初始化"))
		return
	}
	if vars.KnowledgeService == nil {
		resp.ToErrorResponse(errors.New("RAG 服务未初始化，请先完成 AI 配置"))
		return
	}

	ctx := context.Background()
	globalSettings, err := service.NewGlobalSettingsService(ctx).GetGlobalSettings()
	if err != nil || globalSettings == nil {
		resp.ToErrorResponse(errors.New("无法读取全局配置"))
		return
	}

	textDim := uint64(2048)
	if v := utils.PtrIntValue(globalSettings.TextEmbeddingDimension); v > 0 {
		textDim = uint64(v)
	}

	go func() {
		bgCtx := context.Background()
		log.Printf("[ReindexAll] 开始全量重建向量索引，文本维度 %d", textDim)

		// 1. 删除并重建文本集合
		textCollections := []string{
			qdrantx.CollectionMemories,
			qdrantx.CollectionKnowledge,
		}
		for _, col := range textCollections {
			if err := vars.QdrantClient.DeleteCollection(bgCtx, col); err != nil {
				log.Printf("[ReindexAll] 删除集合 %s 失败: %v", col, err)
				return
			}
			if err := vars.QdrantClient.InitCollection(bgCtx, col, textDim); err != nil {
				log.Printf("[ReindexAll] 重建集合 %s 失败: %v", col, err)
				return
			}
		}
		log.Printf("[ReindexAll] 文本集合重建完成")

		// 2. 重建知识库向量
		if err := vars.KnowledgeService.ReindexAll(bgCtx); err != nil {
			log.Printf("[ReindexAll] 知识库重建失败: %v", err)
		}
		log.Printf("[ReindexAll] 知识库向量重建完成")

		log.Printf("[ReindexAll] 全量重建完成")
	}()

	resp.ToResponse("reindex-all started")
}
