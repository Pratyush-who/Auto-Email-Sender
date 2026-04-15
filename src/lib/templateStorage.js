const TEMPLATES_KEY = 'emailauto.templates.v1'

const writeTemplates = (templates) => {
  localStorage.setItem(TEMPLATES_KEY, JSON.stringify(templates))
}

export const loadTemplates = () => {
  try {
    const raw = localStorage.getItem(TEMPLATES_KEY)
    if (!raw) {
      return []
    }

    const parsed = JSON.parse(raw)
    if (!Array.isArray(parsed)) {
      return []
    }

    return parsed
  } catch {
    return []
  }
}

export const createTemplate = (templates, template) => {
  const next = [
    ...templates,
    {
      id: crypto.randomUUID(),
      name: template.name.trim(),
      subject: template.subject,
      body: template.body,
    },
  ]

  writeTemplates(next)
  return next
}

export const updateTemplate = (templates, templateId, template) => {
  const next = templates.map((entry) => {
    if (entry.id !== templateId) {
      return entry
    }

    return {
      ...entry,
      name: template.name.trim(),
      subject: template.subject,
      body: template.body,
    }
  })

  writeTemplates(next)
  return next
}

export const deleteTemplate = (templates, templateId) => {
  const next = templates.filter((entry) => entry.id !== templateId)
  writeTemplates(next)
  return next
}
