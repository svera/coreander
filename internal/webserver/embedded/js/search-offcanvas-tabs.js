"use strict"

function setOffcanvasSearchType(type) {
    const typeInput = document.getElementById("search-offcanvas-type")
    if (typeInput) typeInput.value = type
}

function tabTypeFromEvent(event) {
    const tabBtn = event.target?.closest?.("[data-search-tab]") ?? event.target
    const tabName = tabBtn?.dataset?.searchTab
    return tabName === "authors" || tabName === "documents" ? tabName : null
}

function syncOffcanvasSearchTypeFromPane() {
    const authorPane = document.getElementById("search-offcanvas-authors-panel")
    const type = authorPane?.classList.contains("active") ? "authors" : "documents"
    setOffcanvasSearchType(type)
    return type
}

function setOffcanvasActivePanelInputs(activeType) {
    const docPanel = document.getElementById("search-offcanvas-documents-panel")
    const authorPanel = document.getElementById("search-offcanvas-authors-panel")

    docPanel?.querySelectorAll("input, select, textarea").forEach((el) => {
        if (el.id === "search-offcanvas-type") return
        el.disabled = activeType !== "documents"
    })
    authorPanel?.querySelectorAll("input, select, textarea").forEach((el) => {
        el.disabled = activeType !== "authors"
    })
}

function initSearchOffcanvasTabs() {
    const tabs = document.getElementById("search-offcanvas-tabs")
    if (!tabs) return

    let activeType = syncOffcanvasSearchTypeFromPane()
    setOffcanvasActivePanelInputs(activeType)

    tabs.addEventListener("show.bs.tab", (event) => {
        const tabName = tabTypeFromEvent(event)
        if (!tabName) return
        activeType = tabName
        setOffcanvasSearchType(tabName)
        setOffcanvasActivePanelInputs(tabName)
    })

    tabs.addEventListener("shown.bs.tab", (event) => {
        const tabName = tabTypeFromEvent(event)
        if (!tabName) return
        const sidebarTypeInput = document.getElementById("search-type")
        if (sidebarTypeInput) sidebarTypeInput.value = tabName
        if (window.htmx) {
            window.htmx.trigger(document.body, "update")
        }
    })

    const form = tabs.closest("form")
    form?.addEventListener("submit", () => {
        syncOffcanvasSearchTypeFromPane()
    })
}

initSearchOffcanvasTabs()
