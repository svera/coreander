"use strict"

function setOffcanvasSearchType(type) {
    const typeInput = document.getElementById("search-offcanvas-type")
    if (typeInput) typeInput.value = type
}

function syncOffcanvasSearchTypeFromPane() {
    const authorPane = document.getElementById("search-offcanvas-authors-panel")
    const type = authorPane?.classList.contains("active") ? "authors" : "documents"
    setOffcanvasSearchType(type)
    return type
}

function setOffcanvasActivePanelInputs(activeType) {
    const docPanel    = document.getElementById("search-offcanvas-documents-panel")
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

    const activeType = syncOffcanvasSearchTypeFromPane()
    setOffcanvasActivePanelInputs(activeType)

    // On the search results page search-results-tabs.js handles tab clicks
    if (document.getElementById("search-filters-form")) return

    tabs.addEventListener("click", (event) => {
        const tabBtn = event.target?.closest("[data-search-type-tab]")
        if (!tabBtn) return
        const tabName = tabBtn.dataset.searchTypeTab
        if (tabName !== "authors" && tabName !== "documents") return

        const docPane    = document.getElementById("search-offcanvas-documents-panel")
        const authorPane = document.getElementById("search-offcanvas-authors-panel")
        const toDoc = tabName === "documents"
        docPane?.classList.toggle("show", toDoc)
        docPane?.classList.toggle("active", toDoc)
        authorPane?.classList.toggle("show", !toDoc)
        authorPane?.classList.toggle("active", !toDoc)

        tabs.querySelectorAll("[data-search-type-tab]").forEach(btn => {
            const active = btn.dataset.searchTypeTab === tabName
            btn.classList.toggle("active", active)
            btn.setAttribute("aria-selected", String(active))
        })

        setOffcanvasSearchType(tabName)
        setOffcanvasActivePanelInputs(tabName)
    })

    tabs.closest("form")?.addEventListener("submit", () => {
        syncOffcanvasSearchTypeFromPane()
    })
}

initSearchOffcanvasTabs()
