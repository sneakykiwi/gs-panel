package handlers

import (
	"github.com/sneakykiwi/gs-panel/internal/services"

	"github.com/gofiber/fiber/v3"
)

type TemplateHandler struct {
	templateService *services.TemplateService
}

func NewTemplateHandler(templateService *services.TemplateService) *TemplateHandler {
	return &TemplateHandler{templateService: templateService}
}

func (h *TemplateHandler) List(c fiber.Ctx) error {
	templates := h.templateService.List()
	return c.JSON(templates)
}

func (h *TemplateHandler) Get(c fiber.Ctx) error {
	id := c.Params("id")
	template, ok := h.templateService.Get(id)
	if !ok {
		return fiber.NewError(fiber.StatusNotFound, "Template not found")
	}
	return c.JSON(template)
}

func (h *TemplateHandler) Create(c fiber.Ctx) error {
	var template services.GameTemplate
	if err := c.Bind().JSON(&template); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid template data")
	}

	if err := h.templateService.Add(template); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}

	if err := h.templateService.SaveToFile(template); err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "Template added but failed to save: "+err.Error())
	}

	return c.Status(fiber.StatusCreated).JSON(template)
}

func (h *TemplateHandler) Update(c fiber.Ctx) error {
	id := c.Params("id")
	existing, ok := h.templateService.Get(id)
	if !ok {
		return fiber.NewError(fiber.StatusNotFound, "Template not found")
	}

	if existing.IsBuiltIn {
		return fiber.NewError(fiber.StatusForbidden, "Cannot modify built-in template")
	}

	var template services.GameTemplate
	if err := c.Bind().JSON(&template); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid template data")
	}

	template.ID = id
	if err := h.templateService.Add(template); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}

	if err := h.templateService.SaveToFile(template); err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "Template updated but failed to save: "+err.Error())
	}

	return c.JSON(template)
}

func (h *TemplateHandler) Delete(c fiber.Ctx) error {
	id := c.Params("id")

	if err := h.templateService.Delete(id); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}

	if err := h.templateService.DeleteFile(id); err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "Template deleted but failed to remove file: "+err.Error())
	}

	return c.SendStatus(fiber.StatusNoContent)
}

func (h *TemplateHandler) Reload(c fiber.Ctx) error {
	if err := h.templateService.ReloadTemplates(); err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "Failed to reload templates: "+err.Error())
	}
	return c.JSON(fiber.Map{"message": "Templates reloaded", "count": len(h.templateService.List())})
}
