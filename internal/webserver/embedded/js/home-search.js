"use strict"

import { initDateControls, syncSearchTypeFromPane } from './search-filter-utils.js'

const STORAGE_KEY = "coreander-home-search-tab"

function getStoredTab() {
    try {
        const stored = sessionStorage.getItem(STORAGE_KEY)
        if (stored === "documents" || stored === "authors") {
            return stored
        }
    } catch (_) {
        // ignore storage errors
    }
    return "documents"
}

function setHomeSearchType(type) {
    const typeInput = document.getElementById("home-search-type")
    if (typeInput) typeInput.value = type
}

function setHomeActivePanelInputs(activeType) {
    const docPanel = document.getElementById("home-search-documents-panel")
    const authorPanel = document.getElementById("home-search-authors-panel")

    docPanel?.querySelectorAll("input, select, textarea").forEach((el) => {
        if (el.id === "home-search-type") return
        el.disabled = activeType !== "documents"
    })
    authorPanel?.querySelectorAll("input, select, textarea").forEach((el) => {
        el.disabled = activeType !== "authors"
    })
}

function isAdvancedSearchOpen(collapse) {
    if (collapse?.classList.contains("show")) return true
    const toggle = document.querySelector('[href="#home-advanced-search-collapse"], [data-bs-target="#home-advanced-search-collapse"]')
    return toggle?.getAttribute("aria-expanded") === "true"
}

function resolveSearchType(collapse) {
    if (isAdvancedSearchOpen(collapse)) {
        return syncSearchTypeFromPane("home-search-type", "home-search-authors-panel")
    }
    setHomeSearchType("documents")
    return "documents"
}

function hasFilterParams(params) {
    for (const key of params.keys()) {
        if (key !== "type") return true
    }
    return false
}

function collectPanelParams(panel, composeDateControls, type) {
    if (!panel) return new URLSearchParams()

    composeDateControls()
    const params = new URLSearchParams()
    params.set("type", type)
    panel.querySelectorAll("input, select, textarea").forEach((el) => {
        if (!el.name || el.disabled) return
        if ((el.type === "checkbox" || el.type === "radio") && !el.checked) return
        const value = String(el.value ?? "").trim()
        if (value === "" || value === "0") return
        params.append(el.name, value)
    })
    return params
}

function tabTypeFromEvent(event) {
    const tabBtn = event.target?.closest?.("[data-home-search-tab]") ?? event.target
    const tabName = tabBtn?.dataset?.homeSearchTab
    return tabName === "authors" || tabName === "documents" ? tabName : null
}

function restoreStoredTab() {
    try {
        const stored = sessionStorage.getItem(STORAGE_KEY)
        if (stored !== "authors") return
        const authorsTab = document.getElementById("home-search-tab-authors")
        if (authorsTab && typeof bootstrap !== "undefined") {
            bootstrap.Tab.getOrCreateInstance(authorsTab).show()
        }
    } catch (_) {
        // ignore storage errors
    }
}

function initHomeSearchTabs() {
    const tabs = document.getElementById("home-search-tabs")
    if (!tabs) return

    let activeType = getStoredTab()
    setHomeSearchType(activeType)
    setHomeActivePanelInputs(activeType)

    tabs.addEventListener("show.bs.tab", (event) => {
        const tabName = tabTypeFromEvent(event)
        if (!tabName) return
        activeType = tabName
        setHomeSearchType(tabName)
        setHomeActivePanelInputs(tabName)
    })

    tabs.addEventListener("shown.bs.tab", (event) => {
        const tabName = tabTypeFromEvent(event)
        if (!tabName) return
        try {
            sessionStorage.setItem(STORAGE_KEY, tabName)
        } catch (_) {
            // ignore storage errors
        }
    })

    restoreStoredTab()
    activeType = syncSearchTypeFromPane("home-search-type", "home-search-authors-panel")
    setHomeActivePanelInputs(activeType)
}

function safeInitDateControls(panel, form) {
    if (!panel) return () => {}
    try {
        return initDateControls(panel, form)
    } catch (error) {
        console.error("Error initializing date controls:", error)
        return () => {}
    }
}

function initHomeSearch() {
    const form = document.getElementById("home-search-form")
    if (!form) return

    const collapse = document.getElementById("home-advanced-search-collapse")
    const docPanel = document.getElementById("home-search-documents-panel")
    const authorPanel = document.getElementById("home-search-authors-panel")
    const searchbox = form.querySelector("#searchbox")

    const composeDocumentDates = safeInitDateControls(docPanel, form)
    const composeAuthorDates = safeInitDateControls(authorPanel, form)
    form._coreanderComposeDates = [composeDocumentDates, composeAuthorDates]

    form.addEventListener("submit", (event) => {
        event.preventDefault()
        const query = searchbox?.value.trim() ?? ""
        const searchType = resolveSearchType(collapse)

        if (!isAdvancedSearchOpen(collapse)) {
            if (!query) return
            const params = new URLSearchParams({ type: searchType, search: query })
            window.location.href = "/search?" + params.toString()
            return
        }

        const composeDates = searchType === "authors" ? composeAuthorDates : composeDocumentDates
        const panel = searchType === "authors" ? authorPanel : docPanel

        const params = collectPanelParams(panel, composeDates, searchType)
        if (query) params.set("search", query)
        if (!query && !params.has("name") && !hasFilterParams(params)) return
        window.location.href = "/search?" + params.toString()
    })

    initHomeSearchTabs()

    const docFilters = docPanel?.querySelector("#document-search-filters")
    if (docFilters) {
        import("./document-search-filters.js")
            .then(({ initSubjectsFilters }) => initSubjectsFilters(docFilters, "", null))
            .catch(() => {})
    }
}

initHomeSearch()
