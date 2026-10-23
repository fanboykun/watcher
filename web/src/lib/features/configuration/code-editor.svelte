<script lang="ts">
	import { untrack } from 'svelte';
	import { basicSetup, EditorView } from 'codemirror';
	import { Compartment, EditorState } from '@codemirror/state';
	import {
		HighlightStyle,
		StreamLanguage,
		syntaxHighlighting,
		syntaxTree
	} from '@codemirror/language';
	import { linter } from '@codemirror/lint';
	import { json, jsonParseLinter } from '@codemirror/lang-json';
	import { xml } from '@codemirror/lang-xml';
	import { unifiedMergeView } from '@codemirror/merge';
	import { tags } from '@lezer/highlight';
	import { environmentDiagnostics } from './model';
	let {
		value,
		filename,
		label,
		original = '',
		review = false,
		readonly = false,
		onchange
	}: {
		value: string;
		filename: string;
		label: string;
		original?: string;
		review?: boolean;
		readonly?: boolean;
		onchange: (value: string) => void;
	} = $props();

	const environment = StreamLanguage.define({
		startState: () => ({ quote: '' }),
		token(stream, state) {
			if (state.quote) {
				while (!stream.eol()) {
					const char = stream.next();
					if (char === '\\') stream.next();
					else if (char === state.quote) {
						state.quote = '';
						break;
					}
				}
				return 'string';
			}
			if (stream.eatSpace()) return null;
			if (stream.match(/^#.*/)) return 'comment';
			if (stream.match(/^export\b/)) return 'keyword';
			if (stream.match(/^[A-Za-z_][\w]*(?=\s*=)/)) return 'variableName';
			if (stream.match('=')) return 'operator';
			const char = stream.next();
			if (char === '"' || char === "'") {
				state.quote = char;
				return 'string';
			}
			stream.eatWhile(/[^#'"\s]/);
			return 'string';
		}
	});
	const highlighting = syntaxHighlighting(
		HighlightStyle.define([
			{ tag: tags.comment, color: '#909b91', fontStyle: 'italic' },
			{ tag: tags.string, color: '#b3cf9d' },
			{ tag: [tags.keyword, tags.bool, tags.null], color: '#e9b66a' },
			{ tag: [tags.variableName, tags.propertyName, tags.attributeName], color: '#91c2ce' },
			{ tag: tags.number, color: '#d5aff0' },
			{ tag: tags.invalid, color: '#ef8f88' }
		])
	);
	function language(path: string) {
		if (/\.json$/i.test(path)) return [json(), linter(jsonParseLinter())];
		if (/\.(xml|config|svg)$/i.test(path))
			return [
				xml(),
				linter((view) => {
					const diagnostics: { from: number; to: number; severity: 'error'; message: string }[] =
						[];
					syntaxTree(view.state).iterate({
						enter(node) {
							if (node.type.isError)
								diagnostics.push({
									from: node.from,
									to: node.to,
									severity: 'error',
									message: 'Check XML syntax here.'
								});
						}
					});
					return diagnostics;
				})
			];
		if (/(^|[\\/])\.env(?:\.|$)|\.env$/i.test(path))
			return [environment, linter((view) => environmentDiagnostics(view.state.doc.toString()))];
		return [];
	}
	const theme = EditorView.theme(
		{
			'&': { color: '#e8eae5', backgroundColor: '#121617', fontSize: '12px' },
			'.cm-scroller': { fontFamily: 'var(--font-mono)', lineHeight: '1.8', overflow: 'auto' },
			'.cm-content': { minHeight: '340px', padding: '16px 0' },
			'.cm-line': { padding: '0 16px' },
			'.cm-gutters': { backgroundColor: '#161b1c', color: '#687478', borderColor: '#2b3033' },
			'.cm-activeLine, .cm-activeLineGutter': { backgroundColor: '#ffffff05' },
			'.cm-cursor': { borderLeftColor: '#e9b66a' },
			'&.cm-focused .cm-selectionBackground, .cm-selectionBackground': {
				backgroundColor: '#e9b66a25'
			},
			'.cm-panels': { backgroundColor: '#202628', color: '#e8eae5', borderColor: '#384145' },
			'.cm-textfield': { backgroundColor: '#111719', color: '#e8eae5', borderColor: '#526064' },
			'.cm-button': { backgroundImage: 'none', backgroundColor: '#283033', color: '#e8eae5' },
			'.cm-tooltip': { backgroundColor: '#202628', color: '#e8eae5', borderColor: '#384145' },
			'.cm-changedLine, .cm-insertedLine': { backgroundColor: '#49b88b12' },
			'.cm-deletedChunk': { backgroundColor: '#ef797312' },
			'.cm-insertedText': { backgroundColor: '#49b88b25' },
			'.cm-deletedText': { backgroundColor: '#ef797325' },
			'.cm-changeGutter': { backgroundColor: '#e9b66a' }
		},
		{ dark: true }
	);
	function mount(node: HTMLDivElement) {
		const settings = new Compartment();
		// Keep CodeMirror's permissive newline parser for pasted text. Serialize with
		// the file's original separator so LF clipboard input still edits CRLF files.
		let lineEnding = untrack(() => (value.includes('\r\n') ? '\r\n' : '\n'));
		let externalValue = untrack(() => value);
		let synchronizing = false;
		const contents = () => view.state.doc.sliceString(0, undefined, lineEnding);
		const view = untrack(
			() =>
				new EditorView({
					parent: node,
					state: EditorState.create({
						doc: value,
						extensions: [
							basicSetup,
							theme,
							highlighting,
							settings.of([]),
							EditorView.updateListener.of((update) => {
								if (update.docChanged && !synchronizing) {
									externalValue = contents();
									onchange(externalValue);
								}
							})
						]
					})
				})
		);
		$effect(() => {
			view.dispatch({
				effects: settings.reconfigure([
					language(filename),
					EditorState.readOnly.of(readonly || review),
					EditorView.editable.of(!readonly && !review),
					EditorView.contentAttributes.of({
						'aria-label': label,
						role: 'textbox',
						'aria-multiline': 'true',
						spellcheck: 'false',
						'aria-readonly': String(readonly || review)
					}),
					...(review
						? [
								unifiedMergeView({
									original,
									mergeControls: false,
									syntaxHighlightDeletions: false
								})
							]
						: [])
				])
			});
		});
		$effect(() => {
			const next = value;
			if (externalValue === next) return;
			externalValue = next;
			lineEnding = next.includes('\r\n') ? '\r\n' : '\n';
			// External updates are authoritative. In particular, opening an untouched
			// file with mixed newlines must never emit normalized content back to it.
			if (view.state.doc.toString() === next.replace(/\r\n?|\n/g, '\n')) return;
			synchronizing = true;
			try {
				view.dispatch({
					changes: { from: 0, to: view.state.doc.length, insert: next }
				});
			} finally {
				synchronizing = false;
			}
		});
		return () => view.destroy();
	}
</script>

<div class="code-editor" {@attach mount}></div>
