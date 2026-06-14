"use strict"

import {
    bindOffcanvasFilterSync,
    enableFilterInputsOnPageShow,
    initDateControls,
    initFilterFormBehavior,
    syncSidebarFormToOffcanvas,
} from './search-filter-utils.js'

// Load translations (subjects UI)
let translations = {}
const i18nElement = document.getElementById('i18n')
if (i18nElement) {
    translations = JSON.parse(i18nElement.textContent).i18n
}

function documentsSyncOffcanvas() {
    syncSidebarFormToOffcanvas({
        searchFieldName: 'search',
        offcanvasContainerId: 'document-search-filters',
        afterCopy: (offcanvasContainer) => {
            const sidebarSubjectsHidden = document.getElementById('sidebar-subjects-hidden')
            const offcanvasSubjectsHidden = document.getElementById('subjects-hidden')
            if (sidebarSubjectsHidden && offcanvasSubjectsHidden) {
                offcanvasSubjectsHidden.value = sidebarSubjectsHidden.value
            }
            offcanvasContainer.dispatchEvent(new CustomEvent('syncSubjectsFromHiddenInput'))
        },
    })
}

function initDocumentSearchFilters(searchFilters) {
    if (!searchFilters) return
    const searchFiltersForm = searchFilters.closest('form')
    if (!searchFiltersForm) return

    const idPrefix = searchFilters.id === 'document-search-filters-sidebar' ? 'sidebar-' : ''
    const composeDateControls = initDateControls(searchFilters, searchFiltersForm)

    const { scheduleApplyFilters } = initFilterFormBehavior({
        searchFilters,
        searchFiltersForm,
        composeDateControls,
        listPath: '/documents',
        syncOffcanvas: documentsSyncOffcanvas,
        beforeSidebarApply: () => {
            const sidebarContainer = document.getElementById('document-search-filters-sidebar')
            if (sidebarContainer) sidebarContainer.dispatchEvent(new CustomEvent('syncSubjectsFromHiddenInput'))
        },
    })

    initSubjectsFilters(searchFilters, idPrefix, scheduleApplyFilters)
}

function initSubjectsFilters(searchFilters, idPrefix, triggerSearchUpdate) {
    const subjectsList = document.getElementById(idPrefix + 'subjects-list')
    const subjectsInput = document.getElementById(idPrefix + 'subjects')
    const subjectsHiddenInput = document.getElementById(idPrefix + 'subjects-hidden')
    const subjectsBadgesContainer = document.getElementById(idPrefix + 'subjects-badges-container')
    let selectedSubjectSlugs = []
    let slugToNames = {}
    let nameToSlug = {}

    if (subjectsList) {
        fetch('/subjects')
            .then(response => {
                if (!response.ok) {
                    throw new Error('Failed to fetch subjects')
                }
                return response.json()
            })
            .then(bySlug => {
                slugToNames = {}
                nameToSlug = {}
                subjectsList.innerHTML = ''
                Object.entries(bySlug || {}).forEach(([slug, names]) => {
                    const nameList = names || []
                    slugToNames[slug] = nameList
                    nameList.forEach(name => {
                        nameToSlug[name] = slug
                    })
                    const displayText = nameList.join(', ')
                    nameToSlug[displayText] = slug
                    const option = document.createElement('option')
                    option.value = displayText
                    subjectsList.appendChild(option)
                })
                applyInitialSubjects()
                updateSubjectBadges()
            })
            .catch(error => {
                console.error('Error loading subjects:', error)
            })
    }

    function slugForValue(value) {
        const trimmed = (value || '').trim()
        if (!trimmed) return null
        if (nameToSlug[trimmed]) return nameToSlug[trimmed]
        if (slugToNames[trimmed]) return trimmed
        return null
    }

    function displayNamesForSlug(slug) {
        return (slugToNames[slug] || [slug]).join(', ')
    }

    function updateSubjectBadges() {
        if (!subjectsBadgesContainer || !subjectsHiddenInput) return
        subjectsBadgesContainer.innerHTML = ''
        if (selectedSubjectSlugs.length === 0) {
            subjectsBadgesContainer.classList.add('d-none')
            subjectsHiddenInput.value = ''
            return
        }
        subjectsBadgesContainer.classList.remove('d-none')
        selectedSubjectSlugs.forEach((slug, index) => {
            const displayText = displayNamesForSlug(slug)
            const badge = document.createElement('span')
            badge.className = 'badge rounded-pill text-bg-primary d-inline-flex align-items-center'
            badge.style.pointerEvents = 'all'
            badge.textContent = displayText
            const closeBtn = document.createElement('button')
            closeBtn.type = 'button'
            closeBtn.className = 'btn-close btn-close-white ms-1 mt-0 small'
            const removeSubjectLabel = translations.remove_subject ? translations.remove_subject.replace('%s', displayText) : `Remove subject: ${displayText}`
            closeBtn.setAttribute('aria-label', removeSubjectLabel)
            closeBtn.addEventListener('click', (e) => {
                e.preventDefault()
                e.stopPropagation()
                removeSubject(index)
            })
            badge.appendChild(closeBtn)
            subjectsBadgesContainer.appendChild(badge)
        })
        subjectsHiddenInput.value = selectedSubjectSlugs.join(',')
    }

    function addSubject(value) {
        const slug = slugForValue(value)
        if (!slug) return
        const isDuplicate = selectedSubjectSlugs.includes(slug)
        if (!isDuplicate) {
            selectedSubjectSlugs.push(slug)
            updateSubjectBadges()
            if (triggerSearchUpdate) triggerSearchUpdate()
        }
        if (subjectsInput) subjectsInput.value = ''
    }

    function removeSubject(index) {
        selectedSubjectSlugs.splice(index, 1)
        updateSubjectBadges()
        if (triggerSearchUpdate) triggerSearchUpdate()
        if (subjectsInput) subjectsInput.focus()
    }

    function matchesDatalistOption(value) {
        if (!subjectsList) return false
        const options = Array.from(subjectsList.options)
        return options.some(option => option.value === value)
    }

    function handlePotentialDatalistMatch(value) {
        if (!value) return
        if (matchesDatalistOption(value)) {
            addSubject(value)
        }
    }

    function applyInitialSubjects() {
        if (!subjectsHiddenInput) return
        const raw = subjectsHiddenInput.value
        const parts = raw ? raw.split(',').map(s => s.trim()).filter(Boolean) : []
        const seen = new Set()
        selectedSubjectSlugs = []
        parts.forEach(part => {
            const slug = slugForValue(part) || part
            const key = slug.toLowerCase()
            if (!seen.has(key)) {
                seen.add(key)
                selectedSubjectSlugs.push(slug)
            }
        })
    }

    if (subjectsInput && subjectsHiddenInput) {
        if (document.readyState === 'loading') {
            document.addEventListener('DOMContentLoaded', () => {
                if (Object.keys(slugToNames).length > 0) updateSubjectBadges()
            })
        }
        searchFilters.addEventListener('syncSubjectsFromHiddenInput', () => {
            if (!subjectsHiddenInput) return
            applyInitialSubjects()
            updateSubjectBadges()
        })
        let lastInputValue = ''
        subjectsInput.addEventListener('input', (e) => {
            const value = e.target.value.trim()
            lastInputValue = value
            handlePotentialDatalistMatch(value)
        })
        subjectsInput.addEventListener('change', (e) => {
            const value = e.target.value.trim()
            if (value && value !== lastInputValue) {
                addSubject(value)
            }
        })
        subjectsInput.addEventListener('blur', (e) => {
            handlePotentialDatalistMatch(e.target.value.trim())
        })
        subjectsInput.addEventListener('keydown', (e) => {
            if (e.key === 'Enter') {
                e.preventDefault()
                const value = subjectsInput.value.trim()
                if (value) addSubject(value)
            } else if (e.key === 'Backspace' && subjectsInput.value === '' && selectedSubjectSlugs.length > 0) {
                removeSubject(selectedSubjectSlugs.length - 1)
            }
        })
    }
}

enableFilterInputsOnPageShow(['document-search-filters', 'document-search-filters-sidebar'])

initDocumentSearchFilters(document.getElementById('document-search-filters'))
initDocumentSearchFilters(document.getElementById('document-search-filters-sidebar'))

bindOffcanvasFilterSync({
    sidebarFormId: 'search-filters-form',
    offcanvasContainerId: 'document-search-filters',
    offcanvasElementId: 'search-filters-offcanvas',
    syncOffcanvas: documentsSyncOffcanvas,
})
