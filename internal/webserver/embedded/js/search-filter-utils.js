"use strict"

function isLeapYear(year) {
    return (year % 4 === 0) && (year % 100 !== 0 || year % 400 === 0)
}

export function updateHiddenDateInput(dateControl) {
    const yearInput = dateControl.querySelector('.input-year')
    const monthSelect = dateControl.querySelector('.input-month')
    const dayInput = dateControl.querySelector('.input-day')
    const hiddenDateInput = dateControl.parentElement.querySelector('.date')

    if (!yearInput.value || yearInput.value === '' || yearInput.value === '0') {
        hiddenDateInput.value = ''
        return
    }

    let year = yearInput.value
    if (year.startsWith('-') || year.startsWith('+')) {
        year = year.substring(0, 1) + year.substring(1).padStart(4, '0')
    } else {
        year = year.padStart(4, '0')
    }

    const month = monthSelect.value || '01'
    const day = (dayInput.value || '1').padStart(2, '0')

    hiddenDateInput.value = year + '-' + month + '-' + day
}

function updateMaxDays(monthSelect, dayInput, yearInput, dateControl = null) {
    const month = parseInt(monthSelect.value)
    const year = parseInt(yearInput.value) || new Date().getFullYear()

    let maxDays = 31
    switch (month) {
        case 2:
            maxDays = isLeapYear(year) ? 29 : 28
            break
        case 4:
        case 6:
        case 9:
        case 11:
            maxDays = 30
            break
    }

    dayInput.setAttribute('max', maxDays)

    const currentDay = parseInt(dayInput.value)
    if (currentDay > maxDays) {
        dayInput.value = maxDays
        if (dateControl) {
            updateHiddenDateInput(dateControl)
        }
    }
}

export function copyFormValues(sourceForm, targetForm) {
    for (const el of sourceForm.elements) {
        if (!el.name) continue
        const target = targetForm.elements[el.name]
        if (target && target !== el) {
            if (target.type === 'checkbox' || target.type === 'radio') {
                target.checked = el.checked
            } else {
                target.value = el.value
            }
        }
    }
}

function yearForDisplay(yearStr) {
    if (!yearStr) return ''
    if (yearStr.startsWith('-')) {
        const rest = yearStr.slice(1).replace(/^0+/, '')
        return rest === '' ? '' : '-' + rest
    }
    const stripped = yearStr.replace(/^0+/, '')
    return stripped === '' ? '' : stripped
}

export function applyHiddenDatesToVisible(container) {
    if (!container) return
    container.querySelectorAll('.date-control').forEach(dateControl => {
        const hiddenInput = dateControl.parentElement.querySelector('input.date')
        if (!hiddenInput || !hiddenInput.value) return
        const parts = hiddenInput.value.split('-')
        if (parts.length < 3) return
        const yearInput = dateControl.querySelector('.input-year')
        const monthSelect = dateControl.querySelector('.input-month')
        const dayInput = dateControl.querySelector('.input-day')
        if (yearInput) yearInput.value = yearForDisplay(parts[0])
        if (monthSelect) monthSelect.value = parts[1]
        if (dayInput) dayInput.value = String(parseInt(parts[2], 10))
    })
}

export function initDateControls(searchFilters, searchFiltersForm) {
    searchFilters.querySelectorAll('.date-control').forEach(dateControl => {
        const monthSelect = dateControl.querySelector('.input-month')
        const dayInput = dateControl.querySelector('.input-day')
        const yearInput = dateControl.querySelector('.input-year')
        if (!monthSelect || !dayInput || !yearInput) return

        monthSelect.addEventListener('change', () => {
            updateMaxDays(monthSelect, dayInput, yearInput, dateControl)
            updateHiddenDateInput(dateControl)
        })

        yearInput.addEventListener('change', () => {
            if (parseInt(monthSelect.value) === 2) {
                updateMaxDays(monthSelect, dayInput, yearInput, dateControl)
            }
            updateHiddenDateInput(dateControl)
        })

        yearInput.addEventListener('input', () => {
            updateHiddenDateInput(dateControl)
        })

        dayInput.addEventListener('change', () => {
            updateHiddenDateInput(dateControl)
        })

        dayInput.addEventListener('input', () => {
            updateHiddenDateInput(dateControl)
        })

        updateMaxDays(monthSelect, dayInput, yearInput, dateControl)
        updateHiddenDateInput(dateControl)
    })

    return function composeDateControls() {
        searchFiltersForm.querySelectorAll('.date-control').forEach(el => {
            const yearEl = el.querySelector('.input-year')
            if (!yearEl || yearEl.value === '' || yearEl.value === '0') return
            const composed = el.parentElement.querySelector('.date')
            if (!composed) return
            let year = yearEl.value
            if (year.startsWith('-') || year.startsWith('+')) {
                year = year.substring(0, 1) + year.substring(1).padStart(4, '0')
            } else {
                year = year.padStart(4, '0')
            }
            const month = el.querySelector('.input-month').value || '01'
            const day = (el.querySelector('.input-day').value || '1').padStart(2, '0')
            composed.value = year + '-' + month + '-' + day
        })
    }
}

export function syncSidebarFormToOffcanvas({ searchFieldName, offcanvasContainerId, afterCopy }) {
    const sidebarForm = document.getElementById('search-filters-form')
    const offcanvasContainer = document.getElementById(offcanvasContainerId)
    if (!sidebarForm) return

    const field = sidebarForm.elements[searchFieldName]
    const searchValue = field ? field.value : ''
    const navSearchbox = document.getElementById('searchbox')
    if (navSearchbox) navSearchbox.value = searchValue

    if (!offcanvasContainer) return
    const offcanvasForm = offcanvasContainer.closest('form')
    if (!offcanvasForm) return

    copyFormValues(sidebarForm, offcanvasForm)
    if (afterCopy) afterCopy(offcanvasContainer, sidebarForm)
    applyHiddenDatesToVisible(offcanvasContainer)
}

const FILTER_DEBOUNCE_MS = 600

const SEARCH_LIST_PATHS = new Set(['/search', '/documents', '/authors'])

export function syncSearchTypeFromPane(typeInputId, authorPaneId) {
    const typeInput = document.getElementById(typeInputId)
    const authorPane = document.getElementById(authorPaneId)
    const type = authorPane?.classList.contains('active') ? 'authors' : 'documents'
    if (typeInput) typeInput.value = type
    return type
}

export function syncSidebarSearchTypeFromPane() {
    return syncSearchTypeFromPane('search-type', 'search-sidebar-authors-panel')
}

function composeAllDateControls(form, fallbackCompose) {
    const composers = form?._coreanderComposeDates
    if (composers?.length) {
        composers.forEach((fn) => fn())
        return
    }
    fallbackCompose()
}

function activeSearchListPath(fallbackPath) {
    if (document.getElementById('search-filters-form')) {
        return '/search'
    }
    if (SEARCH_LIST_PATHS.has(window.location.pathname)) {
        return window.location.pathname
    }
    return fallbackPath
}

export function initFilterFormBehavior({
    searchFilters,
    searchFiltersForm,
    composeDateControls,
    listPath,
    syncOffcanvas,
    beforeSidebarApply,
}) {
    const resolvedListPath = activeSearchListPath(listPath)
    const isListPage = SEARCH_LIST_PATHS.has(window.location.pathname) && document.getElementById('search-filters-form')
    let applyingFilters = false

    function applyFilters() {
        applyingFilters = true
        const sidebarForm = document.getElementById('search-filters-form')
        if (sidebarForm && isListPage) {
            syncSidebarSearchTypeFromPane()
            composeAllDateControls(sidebarForm, composeDateControls)
            if (searchFiltersForm !== sidebarForm) {
                copyFormValues(searchFiltersForm, sidebarForm)
                if (beforeSidebarApply) beforeSidebarApply()
            }
            const formData = new FormData(sidebarForm)
            const params = new URLSearchParams()
            for (const [k, v] of formData.entries()) {
                if (v != null && String(v).trim() !== '') params.append(k, v)
            }
            const queryString = params.toString()
            const url = resolvedListPath + (queryString ? '?' + queryString : '')
            history.replaceState(null, '', url)
            window.htmx.trigger(document.body, 'update')
            syncOffcanvas()
        } else {
            composeAllDateControls(searchFiltersForm, composeDateControls)
            const params = new URLSearchParams(new FormData(searchFiltersForm))
            window.location.href = resolvedListPath + '?' + params.toString()
        }
        setTimeout(() => { applyingFilters = false }, 0)
    }

    let scheduleApplyFilters = null
    if (isListPage) {
        let applyFiltersDebounced
        scheduleApplyFilters = function () {
            if (applyingFilters) return
            if (applyFiltersDebounced) clearTimeout(applyFiltersDebounced)
            applyFiltersDebounced = setTimeout(applyFilters, FILTER_DEBOUNCE_MS)
        }

        searchFiltersForm.addEventListener('submit', (e) => {
            e.preventDefault()
            if (applyFiltersDebounced) clearTimeout(applyFiltersDebounced)
            applyFilters()
        })

        searchFiltersForm.addEventListener('input', () => scheduleApplyFilters())
        searchFiltersForm.addEventListener('change', () => scheduleApplyFilters())
    } else {
        searchFiltersForm.addEventListener('submit', () => {
            composeDateControls()
            searchFilters.querySelectorAll('input').forEach(input => {
                if (input.value === '' || input.value === '0') input.setAttribute('disabled', 'disabled')
            })
        })
    }

    searchFiltersForm._coreanderComposeDates = searchFiltersForm._coreanderComposeDates || []
    searchFiltersForm._coreanderComposeDates.push(composeDateControls)

    return { scheduleApplyFilters }
}

export function enableFilterInputsOnPageShow(containerIds) {
    window.addEventListener('pageshow', () => {
        containerIds.forEach(id => {
            const el = document.getElementById(id)
            if (el) {
                el.querySelectorAll('input').forEach(input => {
                    input.removeAttribute('disabled')
                })
            }
        })
    })
}

export function bindOffcanvasFilterSync({ sidebarFormId, offcanvasContainerId, offcanvasElementId, syncOffcanvas }) {
    if (!document.getElementById(sidebarFormId) || !document.getElementById(offcanvasContainerId)) {
        return
    }
    const offcanvasEl = document.getElementById(offcanvasElementId)
    if (offcanvasEl) {
        offcanvasEl.addEventListener('shown.bs.offcanvas', () => syncOffcanvas())
    }
}
