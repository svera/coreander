"use strict"

const STORAGE_KEY = "coreander-home-search-tab"

function activateHomeSearchTab(tabName) {
    document.querySelectorAll("[data-home-search-tab]").forEach((button) => {
        const isActive = button.dataset.homeSearchTab === tabName
        button.classList.toggle("active", isActive)
        button.setAttribute("aria-selected", isActive ? "true" : "false")
    })

    document.querySelectorAll("[data-home-search-panel]").forEach((panel) => {
        const isActive = panel.dataset.homeSearchPanel === tabName
        panel.classList.toggle("d-none", !isActive)
    })

    try {
        sessionStorage.setItem(STORAGE_KEY, tabName)
    } catch (_) {
        // ignore storage errors
    }
}

function initHomeSearchTabs() {
    const tabs = document.getElementById("home-search-tabs")
    if (!tabs) return

    tabs.addEventListener("click", (event) => {
        const button = event.target.closest("[data-home-search-tab]")
        if (!button) return
        activateHomeSearchTab(button.dataset.homeSearchTab)
    })

    let initialTab = "documents"
    try {
        const stored = sessionStorage.getItem(STORAGE_KEY)
        if (stored === "documents" || stored === "authors") {
            initialTab = stored
        }
    } catch (_) {
        // ignore storage errors
    }

    activateHomeSearchTab(initialTab)
}

initHomeSearchTabs()
