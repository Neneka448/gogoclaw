---
name: skill-creator
description: Create new skills, modify and improve existing skills, and measure skill performance. Use when users want to create a skill from scratch, edit, or optimize an existing skill, run evals to test a skill, benchmark skill performance with variance analysis, or optimize a skill's description for better triggering accuracy.
---

# Skill Creator

A skill for creating new skills and iteratively improving them.

## Workspace Skill Storage

All skills in this workspace are stored under `skills/` in the workspace root directory.
Each skill is a folder containing at least a `SKILL.md` file:

```
<workspace>/
└── skills/
    ├── skill-creator/SKILL.md   (this skill)
    ├── my-skill/SKILL.md
    └── another-skill/SKILL.md
```

When creating a new skill, always place it under `skills/<skill-name>/SKILL.md` in the workspace directory. The gogoclaw agent discovers skills via this directory structure and loads them on demand through the built-in `get_skill` tool — the skill body is only read when the agent decides to invoke it, so the description in the YAML frontmatter is the primary trigger signal.

---

At a high level, the process of creating a skill goes like this:

- Decide what you want the skill to do and roughly how it should do it
- Write a draft of the skill
- Create a few test prompts and run the agent-with-skill on them
- Help the user evaluate the results both qualitatively and quantitatively
  - While the runs happen, draft some quantitative evals if there aren't any (if there are some, you can either use as is or modify if you feel something needs to change about them). Then explain them to the user (or if they already existed, explain the ones that already exist)
  - Present results for the user to review and give feedback
- Rewrite the skill based on feedback from the user's evaluation of the results (and also if there are any glaring flaws that become apparent from the quantitative benchmarks)
- Repeat until you're satisfied
- Expand the test set and try again at larger scale

Your job when using this skill is to figure out where the user is in this process and then jump in and help them progress through these stages. So for instance, maybe they're like "I want to make a skill for X". You can help narrow down what they mean, write a draft, write the test cases, figure out how they want to evaluate, run all the prompts, and repeat.

On the other hand, maybe they already have a draft of the skill. In this case you can go straight to the eval/iterate part of the loop.

Of course, you should always be flexible and if the user is like "I don't need to run a bunch of evaluations, just vibe with me", you can do that instead.

Then after the skill is done (but again, the order is flexible), you can also run the skill description improver to optimize the triggering of the skill.

Cool? Cool.

## Communicating with the user

The skill creator is liable to be used by people across a wide range of familiarity with coding jargon. If you haven't heard (and how could you, it's only very recently that it started), there's a trend now where the power of AI is inspiring plumbers to open up their terminals, parents and grandparents to google "how to install npm". On the other hand, the bulk of users are probably fairly computer-literate.

So please pay attention to context cues to understand how to phrase your communication! In the default case, just to give you some idea:

- "evaluation" and "benchmark" are borderline, but OK
- for "JSON" and "assertion" you want to see serious cues from the user that they know what those things are before using them without explaining them

It's OK to briefly explain terms if you're in doubt, and feel free to clarify terms with a short definition if you're unsure if the user will get it.

---

## Creating a skill

### Capture Intent

Start by understanding the user's intent. The current conversation might already contain a workflow the user wants to capture (e.g., they say "turn this into a skill"). If so, extract answers from the conversation history first — the tools used, the sequence of steps, corrections the user made, input/output formats observed. The user may need to fill the gaps, and should confirm before proceeding to the next step.

1. What should this skill enable the agent to do?
2. When should this skill trigger? (what user phrases/contexts)
3. What's the expected output format?
4. Should we set up test cases to verify the skill works? Skills with objectively verifiable outputs (file transforms, data extraction, code generation, fixed workflow steps) benefit from test cases. Skills with subjective outputs (writing style, art) often don't need them. Suggest the appropriate default based on the skill type, but let the user decide.

### Interview and Research

Proactively ask questions about edge cases, input/output formats, example files, success criteria, and dependencies. Wait to write test prompts until you've got this part ironed out.

Use `read_file`, `list_dir`, or `terminal` to explore the workspace and fetch docs if useful for research. Come prepared with context to reduce burden on the user.

### Write the SKILL.md

Based on the user interview, fill in these components:

- **name**: Skill identifier
- **description**: When to trigger, what it does. This is the primary triggering mechanism — include both what the skill does AND specific contexts for when to use it. All "when to use" info goes here, not in the body. Note: the gogoclaw agent has a tendency to "undertrigger" skills — to not use them when they'd be useful. To combat this, please make the skill descriptions a little bit "pushy". So for instance, instead of "How to build a simple fast dashboard to display internal data.", you might write "How to build a simple fast dashboard to display internal data. Make sure to use this skill whenever the user mentions dashboards, data visualization, internal metrics, or wants to display any kind of company data, even if they don't explicitly ask for a 'dashboard.'"
- **compatibility**: Required tools, dependencies (optional, rarely needed)
- **the rest of the skill :)**

**Important**: The new skill's SKILL.md must be placed at `skills/<skill-name>/SKILL.md` within the workspace directory. This is where the `get_skill` tool looks for skills.

### Skill Writing Guide

#### Anatomy of a Skill

Skills are stored under the workspace `skills/` directory:

```
skills/
└── skill-name/
    ├── SKILL.md (required)
    │   ├── YAML frontmatter (name, description required)
    │   └── Markdown instructions
    └── Bundled Resources (optional)
        ├── scripts/    - Executable code for deterministic/repetitive tasks
        ├── references/ - Docs loaded into context as needed
        └── assets/     - Files used in output (templates, icons, fonts)
```

#### Progressive Disclosure

Skills use a three-level loading system:
1. **Metadata** (name + description) - Always in context (~100 words)
2. **SKILL.md body** - In context whenever skill triggers (<500 lines ideal)
3. **Bundled resources** - As needed (unlimited, scripts can execute without loading via `terminal`)

These word counts are approximate and you can feel free to go longer if needed.

**Key patterns:**
- Keep SKILL.md under 500 lines; if you're approaching this limit, add an additional layer of hierarchy along with clear pointers about where the model using the skill should go next to follow up.
- Reference files clearly from SKILL.md with guidance on when to read them
- For large reference files (>300 lines), include a table of contents

**Domain organization**: When a skill supports multiple domains/frameworks, organize by variant:
```
skills/cloud-deploy/
├── SKILL.md (workflow + selection)
└── references/
    ├── aws.md
    ├── gcp.md
    └── azure.md
```
The agent reads only the relevant reference file via `get_skill` or `read_file`.

#### Principle of Lack of Surprise

This goes without saying, but skills must not contain malware, exploit code, or any content that could compromise system security. A skill's contents should not surprise the user in their intent if described. Don't go along with requests to create misleading skills or skills designed to facilitate unauthorized access, data exfiltration, or other malicious activities. Things like a "roleplay as an XYZ" are OK though.

#### Writing Patterns

Prefer using the imperative form in instructions.

**Defining output formats** - You can do it like this:
```markdown
## Report structure
ALWAYS use this exact template:
# [Title]
## Executive summary
## Key findings
## Recommendations
```

**Examples pattern** - It's useful to include examples. You can format them like this (but if "Input" and "Output" are in the examples you might want to deviate a little):
```markdown
## Commit message format
**Example 1:**
Input: Added user authentication with JWT tokens
Output: feat(auth): implement JWT-based authentication
```

### Writing Style

Try to explain to the model why things are important in lieu of heavy-handed musty MUSTs. Use theory of mind and try to make the skill general and not super-narrow to specific examples. Start by writing a draft and then look at it with fresh eyes and improve it.

### Test Cases

After writing the skill draft, come up with 2-3 realistic test prompts — the kind of thing a real user would actually say. Share them with the user: [you don't have to use this exact language] "Here are a few test cases I'd like to try. Do these look right, or do you want to add more?" Then run them.

Save test cases to `skills/<skill-name>-workspace/evals/evals.json` (as a sibling directory to the skill's folder in the workspace `skills/` directory). Don't write assertions yet — just the prompts. You'll draft assertions in the next step while the runs are in progress.

```json
{
  "skill_name": "example-skill",
  "evals": [
    {
      "id": 1,
      "prompt": "User's task prompt",
      "expected_output": "Description of expected result",
      "files": []
    }
  ]
}
```

See `references/schemas.md` (if present in this skill's directory) for the full schema including the `assertions` field, which you'll add later.

## Running and evaluating test cases

This section is one continuous sequence — don't stop partway through.

Put results in `skills/<skill-name>-workspace/` as a sibling to the skill directory under the workspace `skills/` folder. Within the workspace, organize results by iteration (`iteration-1/`, `iteration-2/`, etc.) and within that, each test case gets a directory. Don't create all of this upfront — just create directories as you go.

Example layout:
```
skills/
├── my-skill/
│   └── SKILL.md
└── my-skill-workspace/
    ├── evals/evals.json
    ├── iteration-1/
    │   ├── my-eval-name/
    │   │   ├── with_skill/outputs/
    │   │   └── without_skill/outputs/
    │   └── another-eval-name/
    └── iteration-2/
```

### Step 1: Run all test cases (with-skill AND baseline)

For each test case, run two versions — one with the skill active, one as a baseline. Since gogoclaw has no background subagent spawning, run them sequentially. For each, save output to the appropriate directory.

**With-skill run** — the skill is already deployed in the workspace, so just invoke the agent normally:

```bash
gogoclaw agent --message "<eval prompt>" \
  > skills/<skill-name>-workspace/iteration-<N>/<eval-name>/with_skill/outputs/output.txt 2>&1
```

**Baseline run** (same prompt, baseline depends on context):
- **Creating a new skill**: temporarily rename the skill folder so it's excluded, run the same prompt, then restore. Save to `without_skill/outputs/`.
  ```bash
  mv skills/<skill-name> skills/<skill-name>-disabled
  gogoclaw agent --message "<eval prompt>" \
    > skills/<skill-name>-workspace/iteration-<N>/<eval-name>/without_skill/outputs/output.txt 2>&1
  mv skills/<skill-name>-disabled skills/<skill-name>
  ```
- **Improving an existing skill**: snapshot before editing (`cp -r skills/<skill-name> skills/<skill-name>-workspace/skill-snapshot/`), then point the baseline run at the snapshot. Save to `old_skill/outputs/`.

Write an `eval_metadata.json` for each test case (assertions can be empty for now). Give each eval a descriptive name based on what it's testing — not just "eval-0". Use this name for the directory too. If this iteration uses new or modified eval prompts, create these files for each new eval directory — don't assume they carry over from previous iterations.

```json
{
  "eval_id": 0,
  "eval_name": "descriptive-name-here",
  "prompt": "The user's task prompt",
  "assertions": []
}
```

### Step 2: While runs are in progress, draft assertions

Don't just wait for the runs to finish — you can use this time productively. Draft quantitative assertions for each test case and explain them to the user. If assertions already exist in `evals/evals.json`, review them and explain what they check.

Good assertions are objectively verifiable and have descriptive names — they should read clearly so someone glancing at the results immediately understands what each one checks. Subjective skills (writing style, design quality) are better evaluated qualitatively — don't force assertions onto things that need human judgment.

Update the `eval_metadata.json` files and `evals/evals.json` with the assertions once drafted. Also explain to the user what they'll see — both qualitative outputs and quantitative results.

### Step 3: Capture timing data

After each run completes, record timing data in `timing.json` inside the run directory:

```json
{
  "duration_ms": 23332,
  "total_duration_seconds": 23.3
}
```

Use `date` or check tool output for elapsed time. This matters for comparing run efficiency across iterations.

### Step 4: Grade, aggregate, and present results

Once all runs are done:

1. **Grade each run** — evaluate each assertion against the outputs. Save results to `grading.json` in each run directory. The `expectations` array must use the fields `text`, `passed`, and `evidence` — keep these consistent so results are easy to compare. For assertions that can be checked programmatically, write and run a script via `terminal` rather than eyeballing it — scripts are faster, more reliable, and can be reused across iterations.

   ```json
   {
     "eval_id": 0,
     "config": "with_skill",
     "expectations": [
       {
         "text": "Output contains required section headers",
         "passed": true,
         "evidence": "Found: ## Summary, ## Findings, ## Recommendations"
       }
     ]
   }
   ```

2. **Aggregate results** — compile a summary table across all test cases and configurations. If Python is available via `terminal` and the `skill-creator/scripts/aggregate_benchmark.py` script is present, run it:
   ```bash
   python -m scripts.aggregate_benchmark \
     skills/<skill-name>-workspace/iteration-N \
     --skill-name <name>
   ```
   Otherwise, build a compact markdown summary table manually and present it in the conversation.

3. **Do an analyst pass** — look at the aggregate stats and surface patterns the numbers might hide: assertions that always pass regardless of skill (non-discriminating), high-variance evals (possibly flaky), and time tradeoffs.

4. **Present results to the user** — show outputs for each test case along with the quantitative summary. If the eval viewer script (`skill-creator/eval-viewer/generate_review.py`) is available:
   ```bash
   nohup python skills/skill-creator/eval-viewer/generate_review.py \
     skills/<skill-name>-workspace/iteration-N \
     --skill-name "<name>" \
     --benchmark skills/<skill-name>-workspace/iteration-N/benchmark.json \
     > /dev/null 2>&1 &
   VIEWER_PID=$!
   ```
   For iteration 2+, also pass `--previous-workspace skills/<skill-name>-workspace/iteration-<N-1>`. If there is no display available, use `--static <output_path>` to write a standalone HTML file instead.

   If the viewer isn't available, present results directly in the conversation: for each test case, show the prompt and the output. Ask for feedback inline: "How does this look? Anything you'd change?"

5. **Tell the user** something like: "Here are the results. Take a look and let me know what you think."

### What the user sees

When presenting results, show one test case at a time:
- **Prompt**: the task that was given
- **With-skill output**: what the agent produced with the skill
- **Baseline output**: what it produced without (collapsed or shown side-by-side)
- **Assertion results**: pass/fail for each check
- **Feedback**: ask inline for their thoughts

For iteration 2+, also show the previous iteration's output so the user can compare progress.

### Step 5: Read the feedback

When using the eval viewer, after the user clicks "Submit All Reviews", collect the `feedback.json`:

```json
{
  "reviews": [
    {"run_id": "my-eval-name-with_skill", "feedback": "the output is missing X", "timestamp": "..."},
    {"run_id": "another-eval-name-with_skill", "feedback": "", "timestamp": "..."}
  ],
  "status": "complete"
}
```

Empty feedback means the user thought it was fine. Focus improvements on test cases where the user had specific complaints.

If running the viewer as a background server, kill it when done:
```bash
kill $VIEWER_PID 2>/dev/null
```

---

## Improving the skill

This is the heart of the loop. You've run the test cases, the user has reviewed the results, and now you need to make the skill better based on their feedback.

### How to think about improvements

1. **Generalize from the feedback.** The big picture thing that's happening here is that we're trying to create skills that can be used many times across many different prompts. Here you and the user are iterating on only a few examples over and over again because it helps move faster. The user knows these examples in and out and it's quick for them to assess new outputs. But if the skill you and the user are codeveloping works only for those examples, it's useless. Rather than put in fiddly overfitty changes, or oppressively constrictive MUSTs, if there's some stubborn issue, you might try branching out and using different metaphors, or recommending different patterns of working. It's relatively cheap to try and maybe you'll land on something great.

2. **Keep the prompt lean.** Remove things that aren't pulling their weight. Make sure to read the outputs carefully, not just the final results — if it looks like the skill is causing the agent to waste time doing unproductive things, try removing the parts of the skill driving that behavior and see what happens.

3. **Explain the why.** Try hard to explain the **why** behind everything you're asking the model to do. Today's LLMs are *smart*. They have good theory of mind and when given a good harness can go beyond rote instructions and really make things happen. Even if the feedback from the user is terse or frustrated, try to actually understand the task and why the user is writing what they wrote, and what they actually wrote, and then transmit this understanding into the instructions. If you find yourself writing ALWAYS or NEVER in all caps, or using super rigid structures, that's a yellow flag — if possible, reframe and explain the reasoning so that the model understands why the thing you're asking for is important. That's a more humane, powerful, and effective approach.

4. **Look for repeated work across test cases.** Read the outputs from the test runs and notice if the agent independently wrote similar helper scripts or took the same multi-step approach to something across multiple runs. If all 3 test cases resulted in the agent writing a `process_data.py` or a `build_report.py`, that's a strong signal the skill should bundle that script. Write it once, put it in `skills/<skill-name>/scripts/`, and tell the skill to use it. This saves every future invocation from reinventing the wheel.

This task matters — take your time and really mull things over. Write a draft revision and then look at it with fresh eyes and make improvements. Really do your best to get into the head of the user and understand what they want and need.

### The iteration loop

After improving the skill:

1. Apply your improvements to the skill at `skills/<skill-name>/SKILL.md`
2. Rerun all test cases into a new `iteration-<N+1>/` directory, including baseline runs. If you're creating a new skill, the baseline is always `without_skill` (no skill) — that stays the same across iterations. If you're improving an existing skill, use your judgment on what makes sense as the baseline: the original version the user came in with, or the previous iteration.
3. Present results to the user for review
4. Read the new feedback, improve again, repeat

Keep going until:
- The user says they're happy
- The feedback is all empty (everything looks good)
- You're not making meaningful progress

---

## Advanced: Blind comparison

For situations where you want a more rigorous comparison between two versions of a skill (e.g., the user asks "is the new version actually better?"), run a blind comparison:

1. Run both versions on the same test prompts, saving outputs to separate directories
2. Present both outputs to the user side by side without labeling which version is which
3. Ask them to pick the better one for each case
4. Reveal which was which, then analyze why the winner won

This is optional and most users won't need it. The standard review loop is usually sufficient.

---

## Description Optimization

The description field in SKILL.md frontmatter is the primary mechanism that determines whether the gogoclaw agent invokes a skill. After creating or improving a skill, offer to optimize the description for better triggering accuracy.

### How skill triggering works

Skills appear in the agent's system prompt as a list of available skills with their name + description, and the agent decides whether to call `get_skill` based on that description. The important thing to know is that the agent only consults skills for tasks it can't easily handle on its own — simple, one-step queries like "read this PDF" may not trigger a skill even if the description matches perfectly. Complex, multi-step, or specialized queries reliably trigger skills when the description matches.

This means eval queries for description optimization should be substantive enough that the agent would actually benefit from consulting a skill. Simple queries like "read file X" are poor test cases — they won't trigger skills regardless of description quality.

### Step 1: Generate trigger eval queries

Create 20 eval queries — a mix of should-trigger and should-not-trigger. Save as JSON:

```json
[
  {"query": "the user prompt", "should_trigger": true},
  {"query": "another prompt", "should_trigger": false}
]
```

The queries must be realistic and something a real user would actually type. Not abstract requests, but concrete and specific, with a good amount of detail: file paths, personal context about the user's job or situation, column names, company names, URLs, a little bit of backstory. Some might be lowercase or contain abbreviations, typos, or casual speech. Use a mix of different lengths, and focus on edge cases rather than making them clear-cut.

**Bad**: `"Format this data"`, `"Extract text from file"`, `"Write a report"`

**Good**: `"ok so my boss just sent me this xlsx file (it's in my downloads, called something like 'Q4 sales final FINAL v2.xlsx') and she wants me to add a column that shows the profit margin as a percentage. The revenue is in column C and costs are in column D i think"`

For the **should-trigger** queries (8-10), think about coverage: different phrasings of the same intent — some formal, some casual. Include cases where the user doesn't explicitly name the skill but clearly needs it. Throw in some uncommon use cases and cases where this skill competes with another but should win.

For the **should-not-trigger** queries (8-10), the most valuable are the near-misses — queries that share keywords or concepts with the skill but actually need something different. Think adjacent domains, ambiguous phrasing where a naive keyword match would trigger but shouldn't, and cases where the query touches on something the skill does but in a context where another approach is more appropriate.

The key thing to avoid: don't make should-not-trigger queries obviously irrelevant. They should be genuinely tricky.

### Step 2: Review with user

Present the eval set to the user and let them refine it — add, remove, or adjust the should-trigger classification. Bad eval queries lead to bad descriptions, so this step matters.

### Step 3: Iterate on the description

Test each query by running `gogoclaw agent --message "<query>"` via `terminal` and checking whether the agent calls `get_skill` for your target skill. Then propose a revised description, retest, and converge on the best version.

If the `skill-creator/scripts/run_loop.py` script is available, you can automate this:
```bash
python -m scripts.run_loop \
  --eval-set <path-to-trigger-eval.json> \
  --skill-path skills/<skill-name>/SKILL.md \
  --max-iterations 5 \
  --verbose
```
This handles the full loop: it splits the eval set into 60% train / 40% held-out test, evaluates the current description, proposes improvements, and selects the best description by test score (not train score, to avoid overfitting).

### Step 4: Apply the result

Update the skill's SKILL.md frontmatter with the optimized description. Show the user a before/after and explain what changed and why.

---

## gogoclaw Agent Instructions

The gogoclaw agent doesn't have background subagent spawning, but the core workflow is identical (draft → test → review → improve → repeat). Here's what to adapt:

**Running test cases**: Run each test case sequentially using the `terminal` tool. There's no parallel execution — do them one at a time. This is less rigorous than fully isolated parallel runs, but it's a useful sanity check and the human review step compensates.

**Reviewing results**: If the eval viewer isn't available or can't open a browser, present results directly in the conversation. For each test case, show the prompt and the output inline, and ask for feedback: "How does this look? Anything you'd change?"

**Benchmarking**: Without a dedicated viewer, build a compact markdown table summarizing pass rates across test cases and configurations, and include it in the conversation.

**The iteration loop**: Same as the main loop — improve the skill, rerun test cases, present results for review, read feedback — just without the parallel execution. Organize results into iteration directories on the filesystem as normal.

**Updating an existing skill**: If the user asks you to update an existing skill rather than create a new one:
- Preserve the original name — keep the existing directory name and `name` frontmatter field unchanged.
- Snapshot before editing: `cp -r skills/<skill-name> skills/<skill-name>-workspace/skill-snapshot/`
- Edit the original path directly, then use the snapshot as the baseline for comparisons.

---

## Reference files

The `agents/` directory of this skill (if present) contains instructions for specialized evaluation subagents:
- `agents/grader.md` — How to evaluate assertions against outputs
- `agents/comparator.md` — How to do blind A/B comparison between two outputs
- `agents/analyzer.md` — How to analyze why one version beat another

The `references/` directory (if present) has additional documentation:
- `references/schemas.md` — Full JSON structures for evals.json, grading.json, benchmark.json, etc.

---

Repeating one more time the core loop here for emphasis:

- Figure out what the skill is about
- Draft or edit the skill (stored at `skills/<skill-name>/SKILL.md`)
- Run test prompts with the skill via `gogoclaw agent --message "<prompt>"`
- With the user, evaluate the outputs qualitatively and quantitatively
- Repeat until you and the user are satisfied

Add these steps to your to-do list so you don't forget. Specifically, make sure you always present test case outputs to the user for review before revising the skill yourself — get human eyes on the results first.

Good luck!
