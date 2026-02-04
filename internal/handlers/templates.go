package handlers

import (
	"github.com/sneakykiwi/gs-panel/internal/middleware"
	"github.com/sneakykiwi/gs-panel/internal/services"
	"github.com/sneakykiwi/gs-panel/views/pages/admin"

	"github.com/gofiber/fiber/v3"
	"gopkg.in/yaml.v3"
)

type TemplateHandler struct {
	templateService *services.TemplateService
}

func NewTemplateHandler(templateService *services.TemplateService) *TemplateHandler {
	return &TemplateHandler{templateService: templateService}
}

func (h *TemplateHandler) ListPage(c fiber.Ctx) error {
	user := middleware.GetUser(c)
	templates := h.templateService.List()
	return Render(c, admin.TemplatesPage(user, templates))
}

func (h *TemplateHandler) EditPage(c fiber.Ctx) error {
	user := middleware.GetUser(c)
	id := c.Params("id")

	template, ok := h.templateService.Get(id)
	if !ok {
		return fiber.NewError(fiber.StatusNotFound, "Template not found")
	}

	yamlData, err := yaml.Marshal(template)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "Failed to serialize template")
	}

	return Render(c, admin.TemplateEditPage(user, template, string(yamlData), ""))
}

func (h *TemplateHandler) UpdateYAML(c fiber.Ctx) error {
	user := middleware.GetUser(c)
	id := c.Params("id")

	existing, ok := h.templateService.Get(id)
	if !ok {
		return fiber.NewError(fiber.StatusNotFound, "Template not found")
	}

	if existing.IsBuiltIn {
		return fiber.NewError(fiber.StatusForbidden, "Cannot modify built-in template. Clone it first.")
	}

	yamlContent := c.FormValue("yaml_content")
	if yamlContent == "" {
		return Render(c, admin.TemplateEditPage(user, existing, yamlContent, "YAML content is required"))
	}

	var template services.GameTemplate
	if err := yaml.Unmarshal([]byte(yamlContent), &template); err != nil {
		return Render(c, admin.TemplateEditPage(user, existing, yamlContent, "Invalid YAML: "+err.Error()))
	}

	if template.ID == "" {
		return Render(c, admin.TemplateEditPage(user, existing, yamlContent, "Template ID is required"))
	}
	if template.DockerImage == "" {
		return Render(c, admin.TemplateEditPage(user, existing, yamlContent, "Docker image is required"))
	}

	template.ID = id
	template.IsBuiltIn = false

	if err := h.templateService.Add(template); err != nil {
		return Render(c, admin.TemplateEditPage(user, existing, yamlContent, err.Error()))
	}

	if err := h.templateService.SaveToFile(template); err != nil {
		return Render(c, admin.TemplateEditPage(user, existing, yamlContent, "Failed to save: "+err.Error()))
	}

	return c.Redirect().To("/admin/templates")
}

func (h *TemplateHandler) ClonePage(c fiber.Ctx) error {
	user := middleware.GetUser(c)
	id := c.Params("id")

	template, ok := h.templateService.Get(id)
	if !ok {
		return fiber.NewError(fiber.StatusNotFound, "Template not found")
	}

	cloned := template
	cloned.ID = template.ID + "-custom"
	cloned.Name = template.Name + " (Custom)"
	cloned.IsBuiltIn = false

	yamlData, err := yaml.Marshal(cloned)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "Failed to serialize template")
	}

	return Render(c, admin.TemplateClonePage(user, template, string(yamlData), ""))
}

func (h *TemplateHandler) Clone(c fiber.Ctx) error {
	user := middleware.GetUser(c)
	sourceID := c.Params("id")

	source, ok := h.templateService.Get(sourceID)
	if !ok {
		return fiber.NewError(fiber.StatusNotFound, "Source template not found")
	}

	yamlContent := c.FormValue("yaml_content")
	if yamlContent == "" {
		return Render(c, admin.TemplateClonePage(user, source, yamlContent, "YAML content is required"))
	}

	var template services.GameTemplate
	if err := yaml.Unmarshal([]byte(yamlContent), &template); err != nil {
		return Render(c, admin.TemplateClonePage(user, source, yamlContent, "Invalid YAML: "+err.Error()))
	}

	if template.ID == "" {
		return Render(c, admin.TemplateClonePage(user, source, yamlContent, "Template ID is required"))
	}
	if template.ID == sourceID {
		return Render(c, admin.TemplateClonePage(user, source, yamlContent, "New template must have a different ID"))
	}
	if _, exists := h.templateService.Get(template.ID); exists {
		return Render(c, admin.TemplateClonePage(user, source, yamlContent, "Template with this ID already exists"))
	}
	if template.DockerImage == "" {
		return Render(c, admin.TemplateClonePage(user, source, yamlContent, "Docker image is required"))
	}

	template.IsBuiltIn = false

	if err := h.templateService.Add(template); err != nil {
		return Render(c, admin.TemplateClonePage(user, source, yamlContent, err.Error()))
	}

	if err := h.templateService.SaveToFile(template); err != nil {
		return Render(c, admin.TemplateClonePage(user, source, yamlContent, "Failed to save: "+err.Error()))
	}

	return c.Redirect().To("/admin/templates")
}

func (h *TemplateHandler) CreatePage(c fiber.Ctx) error {
	user := middleware.GetUser(c)

	defaultYAML := `id: my-new-template
name: My New Template
version: "1.0.0"
docker_image: 
default_port: 25565
default_memory: 2048
protocol: both
environment: {}
stop_command: ""
save_command: ""
stop_timeout: 30
`

	return Render(c, admin.TemplateCreatePage(user, defaultYAML, ""))
}

func (h *TemplateHandler) CreateFromYAML(c fiber.Ctx) error {
	user := middleware.GetUser(c)

	yamlContent := c.FormValue("yaml_content")
	if yamlContent == "" {
		return Render(c, admin.TemplateCreatePage(user, yamlContent, "YAML content is required"))
	}

	var template services.GameTemplate
	if err := yaml.Unmarshal([]byte(yamlContent), &template); err != nil {
		return Render(c, admin.TemplateCreatePage(user, yamlContent, "Invalid YAML: "+err.Error()))
	}

	if template.ID == "" {
		return Render(c, admin.TemplateCreatePage(user, yamlContent, "Template ID is required"))
	}
	if _, exists := h.templateService.Get(template.ID); exists {
		return Render(c, admin.TemplateCreatePage(user, yamlContent, "Template with this ID already exists"))
	}
	if template.DockerImage == "" {
		return Render(c, admin.TemplateCreatePage(user, yamlContent, "Docker image is required"))
	}

	template.IsBuiltIn = false

	if err := h.templateService.Add(template); err != nil {
		return Render(c, admin.TemplateCreatePage(user, yamlContent, err.Error()))
	}

	if err := h.templateService.SaveToFile(template); err != nil {
		return Render(c, admin.TemplateCreatePage(user, yamlContent, "Failed to save: "+err.Error()))
	}

	return c.Redirect().To("/admin/templates")
}

func (h *TemplateHandler) DeletePage(c fiber.Ctx) error {
	id := c.Params("id")

	template, ok := h.templateService.Get(id)
	if !ok {
		return fiber.NewError(fiber.StatusNotFound, "Template not found")
	}

	if template.IsBuiltIn {
		return fiber.NewError(fiber.StatusForbidden, "Cannot delete built-in template")
	}

	if err := h.templateService.Delete(id); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}

	if err := h.templateService.DeleteFile(id); err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "Template deleted but failed to remove file: "+err.Error())
	}

	return c.Redirect().To("/admin/templates")
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
