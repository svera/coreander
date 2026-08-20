"use strict"

function isLeapYear(year) {
    return (year % 4 === 0) && (year % 100 !== 0 || year % 400 === 0)
}

function padYear(yearStr) {
    if (yearStr.startsWith('-') || yearStr.startsWith('+')) {
        return yearStr.substring(0, 1) + yearStr.substring(1).padStart(4, '0')
    }
    return yearStr.padStart(4, '0')
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

    const month = monthSelect.value || '01'
    const day = (dayInput.value || '1').padStart(2, '0')

    hiddenDateInput.value = padYear(yearInput.value) + '-' + month + '-' + day
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
        const val = hiddenInput.value
        const isNegative = val.startsWith('-')
        const parts = (isNegative ? val.slice(1) : val).split('-')
        if (parts.length < 3) return
        const yearInput = dateControl.querySelector('.input-year')
        const monthSelect = dateControl.querySelector('.input-month')
        const dayInput = dateControl.querySelector('.input-day')
        if (yearInput) yearInput.value = yearForDisplay((isNegative ? '-' : '') + parts[0])
        if (monthSelect) monthSelect.value = parts[1]
        if (dayInput) dayInput.value = String(parseInt(parts[2], 10))
    })
}

function hasValue(input) {
    return input.value.trim() !== '' && input.value !== '0'
}

function updateDateClearButtonState(dateControl) {
    const yearInput = dateControl.querySelector('.input-year')
    const clearButton = dateControl.querySelector('.clear-date-control')
    if (!yearInput || !clearButton) return
    clearButton.disabled = !hasValue(yearInput)
}

function updateRangeClearButtonState(group) {
    const clearButton = group.querySelector('.clear-range-control')
    if (!clearButton) return
    clearButton.disabled = !Array.from(group.querySelectorAll('input')).some(hasValue)
}

function resetDateControl(dateControl) {
    const yearInput = dateControl.querySelector('.input-year')
    const monthSelect = dateControl.querySelector('.input-month')
    const dayInput = dateControl.querySelector('.input-day')
    if (!yearInput || !monthSelect || !dayInput) return
    yearInput.value = ''
    monthSelect.value = '01'
    dayInput.value = '1'
    updateHiddenDateInput(dateControl)
    updateDateClearButtonState(dateControl)
}

function resetRangeGroup(group) {
    group.querySelectorAll('input').forEach(input => { input.value = '' })
    updateRangeClearButtonState(group)
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
            updateDateClearButtonState(dateControl)
        })

        dayInput.addEventListener('change', () => {
            updateHiddenDateInput(dateControl)
        })

        dayInput.addEventListener('input', () => {
            updateHiddenDateInput(dateControl)
        })

        const clearButton = dateControl.querySelector('.clear-date-control')
        if (clearButton) {
            clearButton.addEventListener('click', (e) => {
                e.preventDefault()
                resetDateControl(dateControl)
                yearInput.dispatchEvent(new Event('input', { bubbles: true }))
            })
        }

        updateMaxDays(monthSelect, dayInput, yearInput, dateControl)
        updateHiddenDateInput(dateControl)
        updateDateClearButtonState(dateControl)
    })

    return {
        compose: function composeDateControls() {
            searchFiltersForm.querySelectorAll('.date-control').forEach(el => {
                const yearEl = el.querySelector('.input-year')
                if (!yearEl || yearEl.value === '' || yearEl.value === '0') return
                const composed = el.parentElement.querySelector('.date')
                if (!composed) return
                const month = el.querySelector('.input-month').value || '01'
                const day = (el.querySelector('.input-day').value || '1').padStart(2, '0')
                composed.value = padYear(yearEl.value) + '-' + month + '-' + day
            })
        },
        reset: function resetDateControls() {
            searchFilters.querySelectorAll('.date-control').forEach(resetDateControl)
        },
    }
}

export function initClearRangeControls(searchFilters) {
    searchFilters.querySelectorAll('.clear-range-control').forEach(clearLink => {
        const group = clearLink.closest('.input-group')
        if (!group) return

        group.querySelectorAll('input').forEach(input => {
            input.addEventListener('input', () => updateRangeClearButtonState(group))
        })

        clearLink.addEventListener('click', (e) => {
            e.preventDefault()
            resetRangeGroup(group)
            const firstInput = group.querySelector('input')
            if (firstInput) firstInput.dispatchEvent(new Event('input', { bubbles: true }))
        })

        updateRangeClearButtonState(group)
    })

    return function resetRangeControls() {
        searchFilters.querySelectorAll('.clear-range-control').forEach(clearLink => {
            const group = clearLink.closest('.input-group')
            if (group) resetRangeGroup(group)
        })
    }
}

function hasActiveFilters(searchFilters) {
    const isInDateControl = el => el.closest('.date-control')

    const hasTextOrNumberFilter = Array.from(
        searchFilters.querySelectorAll('input[type="search"], input[type="text"], input[type="number"]')
    ).some(el => !isInDateControl(el) && hasValue(el))
    if (hasTextOrNumberFilter) return true

    const hasDateFilter = Array.from(searchFilters.querySelectorAll('.date-control .input-year')).some(hasValue)
    if (hasDateFilter) return true

    const hasSelectFilter = Array.from(searchFilters.querySelectorAll('select')).some(el => !isInDateControl(el) && el.value !== '')
    if (hasSelectFilter) return true

    const hasCheckboxFilter = Array.from(searchFilters.querySelectorAll('input[type="checkbox"]')).some(el => el.checked)
    if (hasCheckboxFilter) return true

    const subjectsHiddenInput = searchFilters.querySelector('input[name="subjects"]')
    return Boolean(subjectsHiddenInput && subjectsHiddenInput.value.trim() !== '')
}

export function initClearAllFilters(searchFilters, searchFiltersForm, { resetDateControls, resetRangeControls } = {}) {
    const clearAllButton = searchFilters.querySelector('.clear-all-filters')
    if (!clearAllButton) return

    function updateClearAllButtonState() {
        clearAllButton.disabled = !hasActiveFilters(searchFilters)
    }

    searchFilters.addEventListener('input', updateClearAllButtonState)
    searchFilters.addEventListener('change', updateClearAllButtonState)
    new MutationObserver(updateClearAllButtonState).observe(searchFilters, { childList: true, subtree: true })

    clearAllButton.addEventListener('click', (e) => {
        e.preventDefault()

        searchFilters.querySelectorAll('input[type="search"], input[type="text"]').forEach(input => {
            input.value = ''
        })
        searchFilters.querySelectorAll('select').forEach(select => {
            select.value = ''
        })
        searchFilters.querySelectorAll('input[type="checkbox"]').forEach(checkbox => {
            checkbox.checked = false
        })
        resetDateControls?.()
        resetRangeControls?.()

        const subjectsHiddenInput = searchFilters.querySelector('input[name="subjects"]')
        if (subjectsHiddenInput) subjectsHiddenInput.value = ''
        searchFilters.dispatchEvent(new CustomEvent('syncSubjectsFromHiddenInput'))

        searchFiltersForm.dispatchEvent(new Event('input', { bubbles: true }))
        updateClearAllButtonState()
    })

    updateClearAllButtonState()
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
    offcanvasContainer.dispatchEvent(new Event('input'))
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

export function tabTypeFromEvent(event, attrSelector, datasetKey) {
    const tabBtn = event.target?.closest?.(attrSelector) ?? event.target
    const tabName = tabBtn?.dataset?.[datasetKey]
    return tabName === "authors" || tabName === "documents" ? tabName : null
}

export function setActivePanelInputs(activeType, docPanelId, authorPanelId, typeInputId) {
    const typeInput = document.getElementById(typeInputId)
    if (typeInput) typeInput.value = activeType
    const docPanel = document.getElementById(docPanelId)
    const authorPanel = document.getElementById(authorPanelId)
    docPanel?.querySelectorAll("input, select, textarea").forEach(el => {
        if (el.id === typeInputId) return
        el.disabled = activeType !== "documents"
    })
    authorPanel?.querySelectorAll("input, select, textarea").forEach(el => {
        el.disabled = activeType !== "authors"
    })
}

function composeAllDateControls(form) {
    form._coreanderComposeDates?.forEach(fn => fn())
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
            composeAllDateControls(sidebarForm)
            if (searchFiltersForm !== sidebarForm) {
                copyFormValues(searchFiltersForm, sidebarForm)
                if (beforeSidebarApply) beforeSidebarApply()
            }
            const formData = new FormData(sidebarForm)
            const params = new URLSearchParams()
            for (const [k, v] of formData.entries()) {
                if (v != null && String(v).trim() !== '') params.append(k, v)
            }
            const sortEl = document.getElementById('sort-by')
            if (sortEl && sortEl.value && !params.has('sort-by')) {
                params.append('sort-by', sortEl.value)
            }
            const queryString = params.toString()
            const url = resolvedListPath + (queryString ? '?' + queryString : '')
            history.replaceState(null, '', url)
            window.htmx.trigger(document.body, 'update')
            syncOffcanvas()
        } else {
            composeAllDateControls(searchFiltersForm)
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
        searchFiltersForm.addEventListener('change', (e) => {
            if (e.target.tagName === 'SELECT' || e.target.type === 'checkbox') {
                if (applyFiltersDebounced) clearTimeout(applyFiltersDebounced)
                applyFilters()
            } else {
                scheduleApplyFilters()
            }
        })
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

    if (isListPage) {
        document.getElementById('search-filters-form')._coreanderApplyFilters = applyFilters
    }

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

export function initSearchFilters(searchFilters, { syncOffcanvas, beforeSidebarApply, onInit } = {}) {
    if (!searchFilters) return
    const searchFiltersForm = searchFilters.closest('form')
    if (!searchFiltersForm) return

    const { compose: composeDateControls, reset: resetDateControls } = initDateControls(searchFilters, searchFiltersForm)
    const resetRangeControls = initClearRangeControls(searchFilters)
    initClearAllFilters(searchFilters, searchFiltersForm, { resetDateControls, resetRangeControls })

    if (searchFiltersForm.dataset.coreanderFilterBehavior === 'true') {
        searchFiltersForm._coreanderComposeDates = searchFiltersForm._coreanderComposeDates || []
        searchFiltersForm._coreanderComposeDates.push(composeDateControls)
        onInit?.(null)
        return
    }

    const { scheduleApplyFilters } = initFilterFormBehavior({
        searchFilters,
        searchFiltersForm,
        composeDateControls,
        listPath: '/search',
        syncOffcanvas,
        beforeSidebarApply,
    })
    searchFiltersForm.dataset.coreanderFilterBehavior = 'true'
    onInit?.(scheduleApplyFilters)
}
