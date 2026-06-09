"use strict"

function isLeapYear(year) {
    return (year % 4 === 0) && (year % 100 !== 0 || year % 400 === 0)
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

function updateHiddenDateInput(dateControl) {
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

function copyFormValues(sourceForm, targetForm) {
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

function applyHiddenDatesToVisible(container) {
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

function syncSidebarFormToOffcanvas() {
    const sidebarForm = document.getElementById('search-filters-form')
    const offcanvasContainer = document.getElementById('author-search-filters')
    if (!sidebarForm) return

    const searchValue = sidebarForm.elements['name'] ? sidebarForm.elements['name'].value : ''
    const navSearchbox = document.getElementById('searchbox')
    if (navSearchbox) navSearchbox.value = searchValue

    if (!offcanvasContainer) return
    const offcanvasForm = offcanvasContainer.closest('form')
    if (!offcanvasForm) return

    copyFormValues(sidebarForm, offcanvasForm)
    applyHiddenDatesToVisible(offcanvasContainer)
}

function initAuthorSearchFilters(searchFilters) {
    if (!searchFilters) return
    const searchFiltersForm = searchFilters.closest('form')
    if (!searchFiltersForm) return

    searchFilters.querySelectorAll('.date-control').forEach(dateControl => {
        const monthSelect = dateControl.querySelector('.input-month')
        const dayInput = dateControl.querySelector('.input-day')
        const yearInput = dateControl.querySelector('.input-year')

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

    function composeDateControls() {
        searchFiltersForm.querySelectorAll('.date-control').forEach(function (el) {
            const yearEl = el.querySelector('.input-year')
            if (!yearEl || (yearEl.value === '' || yearEl.value === '0')) return
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

    const isAuthorsPage = window.location.pathname === '/authors'
    let applyingFilters = false

    function applyFilters() {
        applyingFilters = true
        composeDateControls()
        const sidebarForm = document.getElementById('search-filters-form')
        if (sidebarForm && isAuthorsPage) {
            if (searchFiltersForm !== sidebarForm) {
                copyFormValues(searchFiltersForm, sidebarForm)
            }
            const formData = new FormData(sidebarForm)
            const params = new URLSearchParams()
            for (const [k, v] of formData.entries()) {
                if (v != null && String(v).trim() !== '') params.append(k, v)
            }
            const queryString = params.toString()
            const url = '/authors' + (queryString ? '?' + queryString : '')
            window.htmx.trigger(document.body, 'update')
            history.replaceState(null, '', url)
            syncSidebarFormToOffcanvas()
        } else {
            const params = new URLSearchParams(new FormData(searchFiltersForm))
            window.location.href = '/authors?' + params.toString()
        }
        setTimeout(() => { applyingFilters = false }, 0)
    }

    const FILTER_DEBOUNCE_MS = 600
    let applyFiltersDebounced

    function scheduleApplyFilters() {
        if (applyingFilters) return
        if (applyFiltersDebounced) clearTimeout(applyFiltersDebounced)
        applyFiltersDebounced = setTimeout(applyFilters, FILTER_DEBOUNCE_MS)
    }

    if (isAuthorsPage) {
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
}

window.addEventListener('pageshow', () => {
    ['author-search-filters', 'author-search-filters-sidebar'].forEach(id => {
        const el = document.getElementById(id)
        if (el) {
            el.querySelectorAll('input').forEach(input => {
                input.removeAttribute('disabled')
            })
        }
    })
})

initAuthorSearchFilters(document.getElementById('author-search-filters'))
initAuthorSearchFilters(document.getElementById('author-search-filters-sidebar'))

if (document.getElementById('search-filters-form') && document.getElementById('author-search-filters')) {
    const offcanvasEl = document.getElementById('search-filters-offcanvas')
    if (offcanvasEl) {
        offcanvasEl.addEventListener('shown.bs.offcanvas', () => syncSidebarFormToOffcanvas())
    }
}
