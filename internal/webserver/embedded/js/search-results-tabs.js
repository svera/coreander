"use strict"

function switchOffcanvasPanes(tabName) {
    const docPane = document.getElementById("search-offcanvas-documents-panel")
    const authorPane = document.getElementById("search-offcanvas-authors-panel")
    const docBtn = document.querySelector("#search-offcanvas-tabs [data-search-tab='documents']")
    const authorBtn = document.querySelector("#search-offcanvas-tabs [data-search-tab='authors']")
    const typeInput = document.getElementById("search-offcanvas-type")

    const toDoc = tabName === "documents"
    docPane?.classList.toggle("show", toDoc)
    docPane?.classList.toggle("active", toDoc)
    authorPane?.classList.toggle("show", !toDoc)
    authorPane?.classList.toggle("active", !toDoc)
    docBtn?.classList.toggle("active", toDoc)
    docBtn?.setAttribute("aria-selected", String(toDoc))
    authorBtn?.classList.toggle("active", !toDoc)
    authorBtn?.setAttribute("aria-selected", String(!toDoc))
    if (typeInput) typeInput.value = tabName

    docPane?.querySelectorAll("input, select, textarea").forEach(el => {
        if (el.id === "search-offcanvas-type") return
        el.disabled = !toDoc
    })
    authorPane?.querySelectorAll("input, select, textarea").forEach(el => {
        el.disabled = toDoc
    })
}

document.addEventListener("click", (event) => {
    const tabBtn = event.target?.closest("[data-search-type-tab]")
    if (!tabBtn) return
    const tabName = tabBtn.dataset.searchTypeTab
    if (tabName !== "documents" && tabName !== "authors") return

    const sidebarTabId = tabName === "authors" ? "search-sidebar-tab-authors" : "search-sidebar-tab-documents"
    const sidebarTab = document.getElementById(sidebarTabId)
    if (sidebarTab && typeof bootstrap !== "undefined") {
        bootstrap.Tab.getOrCreateInstance(sidebarTab).show()
    }

    switchOffcanvasPanes(tabName)
})
