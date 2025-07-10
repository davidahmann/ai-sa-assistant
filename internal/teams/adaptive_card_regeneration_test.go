// Copyright 2024 AI SA Assistant Project
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package teams

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGenerateRegenerationOptionsCard(t *testing.T) {
	query := "How do I migrate to AWS?"
	responseID := "resp_123"
	previousResponse := "Previous response about AWS migration"

	cardJSON, err := GenerateRegenerationOptionsCard(query, responseID, previousResponse)
	require.NoError(t, err)
	assert.NotEmpty(t, cardJSON)

	// Parse the card to verify structure
	var card AdaptiveCard
	err = json.Unmarshal([]byte(cardJSON), &card)
	require.NoError(t, err)

	// Verify card properties
	assert.Equal(t, "AdaptiveCard", card.Type)
	assert.Equal(t, "http://adaptivecards.io/schemas/adaptive-card.json", card.Schema)
	assert.Equal(t, "1.5", card.Version)

	// Verify body elements
	require.GreaterOrEqual(t, len(card.Body), 2)
	assert.Equal(t, "TextBlock", card.Body[0].Type)
	assert.Contains(t, card.Body[0].Text, "🔄 Regenerate Response")

	// Verify actions (5 presets + 1 cancel = 6 actions)
	assert.Len(t, card.Actions, 6)

	// Verify preset actions
	expectedTitles := []string{"More Creative", "Balanced", "More Focused", "More Detailed", "More Concise"}
	expectedPresets := []string{"creative", "balanced", "focused", "detailed", "concise"}
	presetActions := card.Actions[:5]
	for i, action := range presetActions {
		assert.Equal(t, "Action.Http", action.Type)
		assert.Equal(t, "POST", action.Method)
		assert.Equal(t, "/teams-regenerate", action.URL)
		assert.Contains(t, action.Title, expectedTitles[i])

		// Verify action body
		require.NotNil(t, action.Body)
		assert.Equal(t, query, action.Body["query"])
		assert.Equal(t, responseID, action.Body["response_id"])
		assert.Equal(t, previousResponse, action.Body["previous_response"])
		assert.Equal(t, "regenerate", action.Body["action"])
		assert.Equal(t, expectedPresets[i], action.Body["preset"])
	}

	// Verify cancel action
	cancelAction := card.Actions[5]
	assert.Equal(t, "Action.Http", cancelAction.Type)
	assert.Contains(t, cancelAction.Title, "Cancel")
	assert.Equal(t, "/teams-feedback", cancelAction.URL)
}

func TestGenerateComparisonCard(t *testing.T) {
	query := "How do I migrate to AWS?"
	originalResponse := "Original response about AWS migration"
	regeneratedResponse := "Regenerated response about AWS migration"
	preset := "creative"

	cardJSON, err := GenerateComparisonCard(query, originalResponse, regeneratedResponse, preset)
	require.NoError(t, err)
	assert.NotEmpty(t, cardJSON)

	// Parse the card to verify structure
	var card AdaptiveCard
	err = json.Unmarshal([]byte(cardJSON), &card)
	require.NoError(t, err)

	// Verify card properties
	assert.Equal(t, "AdaptiveCard", card.Type)
	assert.Equal(t, "1.5", card.Version)

	// Verify body contains query, preset info, and both responses
	bodyText := ""
	for _, element := range card.Body {
		if element.Type == "TextBlock" {
			bodyText += element.Text + " "
		}
	}

	assert.Contains(t, bodyText, query)
	assert.Contains(t, bodyText, "Creative preset") // The preset is capitalized and includes "preset"
	assert.Contains(t, bodyText, regeneratedResponse)
	assert.Contains(t, bodyText, originalResponse)

	// Verify actions (Better, Worse, Try Another = 3 actions)
	assert.Len(t, card.Actions, 3)

	actionTitles := []string{}
	for _, action := range card.Actions {
		actionTitles = append(actionTitles, action.Title)
		assert.Equal(t, "Action.Http", action.Type)
		assert.Equal(t, "POST", action.Method)
	}

	assert.Contains(t, actionTitles, "👍 Better")
	assert.Contains(t, actionTitles, "👎 Worse")
	assert.Contains(t, actionTitles, "🔄 Try Another")
}

func TestGenerateComparisonCardEmptyOriginal(t *testing.T) {
	query := "How do I migrate to AWS?"
	originalResponse := ""
	regeneratedResponse := "Regenerated response about AWS migration"
	preset := "focused"

	cardJSON, err := GenerateComparisonCard(query, originalResponse, regeneratedResponse, preset)
	require.NoError(t, err)
	assert.NotEmpty(t, cardJSON)

	// Parse the card to verify structure
	var card AdaptiveCard
	err = json.Unmarshal([]byte(cardJSON), &card)
	require.NoError(t, err)

	// Verify that original response section is not included when empty
	bodyText := ""
	for _, element := range card.Body {
		if element.Type == "TextBlock" {
			bodyText += element.Text + " "
		}
	}

	assert.Contains(t, bodyText, regeneratedResponse)
	assert.NotContains(t, bodyText, "Previous Response:")
}

func TestAdaptiveCardJSONStructure(t *testing.T) {
	query := "Test query"
	responseID := "resp_test"
	previousResponse := "Test previous response"

	cardJSON, err := GenerateRegenerationOptionsCard(query, responseID, previousResponse)
	require.NoError(t, err)

	// Verify it's valid JSON
	var jsonData map[string]interface{}
	err = json.Unmarshal([]byte(cardJSON), &jsonData)
	require.NoError(t, err)

	// Verify required Adaptive Card fields
	assert.Equal(t, "AdaptiveCard", jsonData["type"])
	assert.Equal(t, "http://adaptivecards.io/schemas/adaptive-card.json", jsonData["$schema"])
	assert.Equal(t, "1.5", jsonData["version"])
	assert.Contains(t, jsonData, "body")
	assert.Contains(t, jsonData, "actions")

	// Verify body is an array
	body, ok := jsonData["body"].([]interface{})
	require.True(t, ok)
	assert.Greater(t, len(body), 0)

	// Verify actions is an array
	actions, ok := jsonData["actions"].([]interface{})
	require.True(t, ok)
	assert.Greater(t, len(actions), 0)
}

func TestRegenerationCardPresetEmojis(t *testing.T) {
	query := "Test query"
	responseID := "resp_test"
	previousResponse := "Test response"

	cardJSON, err := GenerateRegenerationOptionsCard(query, responseID, previousResponse)
	require.NoError(t, err)

	var card AdaptiveCard
	err = json.Unmarshal([]byte(cardJSON), &card)
	require.NoError(t, err)

	// Verify each preset has the correct emoji
	expectedEmojis := map[string]string{
		"creative": "🎨",
		"balanced": "⚖️",
		"focused":  "🎯",
		"detailed": "📚",
		"concise":  "⚡",
	}

	presetActions := card.Actions[:5] // First 5 are preset actions
	for _, action := range presetActions {
		preset := action.Body["preset"].(string)
		expectedEmoji := expectedEmojis[preset]
		assert.Contains(t, action.Title, expectedEmoji)
	}
}
