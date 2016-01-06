/*
TODO:
- validate input on client and server
- specify ranges for ingredients
- return useful error messages for invalid ingredients
*/
package gohome

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"reflect"
	"time"
)

//TODO: make internal
type RecipeManager struct {
	CookBooks []*CookBook
	System    *System
	Recipes   []*Recipe

	dataPath       string
	eventBroker    EventBroker
	triggerFactory map[string]func() Trigger
	actionFactory  map[string]func() Action
}

func (rm *RecipeManager) Init(eb EventBroker, dataPath string) error {
	rm.eventBroker = eb
	rm.dataPath = dataPath
	rm.CookBooks = loadCookBooks(dataPath)
	rm.triggerFactory = buildTriggerFactory(rm.CookBooks)
	rm.actionFactory = buildActionFactory(rm.CookBooks)

	recipes, err := rm.loadRecipes(dataPath)
	if err != nil {
		return err
	}

	rm.Recipes = recipes
	for _, recipe := range recipes {
		rm.RegisterAndStart(recipe)
	}
	return nil
}

func (rm *RecipeManager) RegisterAndStart(r *Recipe) {
	rm.eventBroker.AddConsumer(r.Trigger.(EventConsumer))
	r.Start()
}

func (rm *RecipeManager) UnregisterAndStop(r *Recipe) {
	rm.eventBroker.RemoveConsumer(r.Trigger.(EventConsumer))
	r.Stop()
}

func (rm *RecipeManager) RecipeByID(id string) *Recipe {
	for _, recipe := range rm.Recipes {
		if recipe.Identifiable.ID == id {
			return recipe
		}
	}
	return nil
}

func (rm *RecipeManager) EnableRecipe(r *Recipe, enabled bool) error {
	oldEnabled := r.Trigger.Enabled()
	if oldEnabled == enabled {
		return nil
	}

	r.Trigger.SetEnabled(enabled)
	return rm.SaveRecipe(r, false)
}

type recipeJSON struct {
	Identifiable Identifiable
	Enabled      bool `json:"enabled"`
	Trigger      triggerWrapper
	Action       actionWrapper
}

type triggerWrapper struct {
	Type    string                 `json:"type"`
	Trigger map[string]interface{} `json:"fields"`
}

type actionWrapper struct {
	Type   string                 `json:"type"`
	Action map[string]interface{} `json:"fields"`
}

func (rm *RecipeManager) UnmarshalNewRecipe(data map[string]interface{}) (*Recipe, error) {
	_, ok := data["name"]
	if !ok {
		return nil, errors.New("Missing name key")
	}
	name, ok := data["name"].(string)
	if !ok {
		return nil, errors.New("Invalid value for name, must be a string")
	}

	_, ok = data["description"]
	if !ok {
		return nil, errors.New("Missing description key")
	}
	desc, ok := data["description"].(string)
	if !ok {
		return nil, errors.New("Invalid value for description, must be a string")
	}

	_, ok = data["trigger"]
	if !ok {
		return nil, errors.New("Missing trigger key")
	}
	triggerData, ok := data["trigger"].(map[string]interface{})
	if !ok {
		return nil, errors.New("Invalid value for trigger, must be an object")
	}

	_, ok = triggerData["id"]
	if !ok {
		return nil, errors.New("Missing id key in trigger object")
	}
	triggerID, ok := triggerData["id"].(string)
	if !ok {
		return nil, errors.New("Invalid value, trigger.id must be a string")
	}

	_, ok = triggerData["ingredients"]
	if !ok {
		return nil, errors.New("Missing trigger.ingredients key")
	}
	triggerIngredients, ok := triggerData["ingredients"].(map[string]interface{})
	if !ok {
		return nil, errors.New("Invalid value for trigger.ingredients, must be an object")
	}

	_, ok = rm.triggerFactory[triggerID]
	if !ok {
		return nil, errors.New(fmt.Sprintf("Invalid trigger ID: %s", triggerID))
	}

	_, ok = data["action"]
	if !ok {
		return nil, errors.New("Missing action key")
	}
	actionData, ok := data["action"].(map[string]interface{})
	if !ok {
		return nil, errors.New("Invalid value for action, must be an object")
	}
	_, ok = actionData["id"]
	if !ok {
		return nil, errors.New("Missing id key in action object")
	}
	actionID, ok := actionData["id"].(string)
	if !ok {
		return nil, errors.New("Invalid value, action.id must be a string")
	}

	_, ok = actionData["ingredients"]
	if !ok {
		return nil, errors.New("Missing action.ingredients key")
	}
	actionIngredients, ok := actionData["ingredients"].(map[string]interface{})
	if !ok {
		return nil, errors.New("Invalid value for action.ingredients, must be an object")
	}

	_, ok = rm.actionFactory[actionID]
	if !ok {
		return nil, errors.New(fmt.Sprintf("Invalid action ID: %s", actionID))
	}

	trigger := rm.triggerFactory[triggerID]()
	action := rm.actionFactory[actionID]()

	err := setIngredients(trigger, triggerIngredients, reflect.ValueOf(trigger).Elem())
	if err != nil {
		return nil, err
	}
	err = setIngredients(action, actionIngredients, reflect.ValueOf(action).Elem())
	if err != nil {
		return nil, err
	}

	enabled := true
	recipe, err := NewRecipe(name, desc, enabled, trigger, action, rm.System)
	return recipe, err
}

func (rm *RecipeManager) SaveRecipe(r *Recipe, appendTo bool) error {
	// Since Trigger and Action are interfaces, we need to also save the underlying
	// concrete type to the JSON file so we can unmarshal to the correct type later

	out := recipeJSON{}
	out.Identifiable = r.Identifiable
	out.Enabled = r.Trigger.Enabled()

	//err := setIngredients(trigger, triggerIngredients, reflect.ValueOf(trigger).Elem())
	out.Trigger = triggerWrapper{Type: r.Trigger.Type(), Trigger: getIngredientValueMap(r.Trigger, reflect.ValueOf(r.Trigger).Elem())}
	out.Action = actionWrapper{Type: r.Action.Type(), Action: getIngredientValueMap(r.Action, reflect.ValueOf(r.Action).Elem())}

	b, err := json.Marshal(out)
	if err != nil {
		return err
	}

	err = ioutil.WriteFile(rm.recipePath(r), b, 0644)
	if err != nil {
		return err
	}

	if appendTo {
		rm.Recipes = append(rm.Recipes, r)
	}
	return nil
}

func (rm *RecipeManager) recipePath(r *Recipe) string {
	return filepath.Join(rm.dataPath, r.Identifiable.ID+".json")
}

func (rm *RecipeManager) DeleteRecipe(r *Recipe) error {

	p := rm.recipePath(r)
	fmt.Println(p)
	err := os.Remove(rm.recipePath(r))
	if err != nil {
		return err
	}

	for i, recipe := range rm.Recipes {
		if recipe.Identifiable.ID == r.Identifiable.ID {
			rm.Recipes = append(rm.Recipes[:i], rm.Recipes[i+1:]...)
			//			rm.Recipes[len(rm.Recipes)-1] = nil
			break
		}
	}

	rm.UnregisterAndStop(r)
	return nil
}

func getIngredientValueMap(i Ingredientor, v reflect.Value) map[string]interface{} {
	values := make(map[string]interface{})
	for _, ingredient := range i.Ingredients() {
		// Want to store duration as ms, so need to massage
		var value interface{}
		if ingredient.Type == "duration" {
			value = int64(time.Duration(v.FieldByName(ingredient.Identifiable.ID).Int()) / time.Millisecond)
		} else {
			value = v.FieldByName(ingredient.Identifiable.ID).Interface()
		}
		values[ingredient.Identifiable.ID] = value
	}
	return values
}

func (rm *RecipeManager) loadRecipes(path string) ([]*Recipe, error) {
	files, err := ioutil.ReadDir(path)
	if err != nil {
		return nil, err
	}

	recipes := make([]*Recipe, 0)
	for _, fileInfo := range files {
		filepath := filepath.Join(path, fileInfo.Name())
		recipe, err := rm.loadRecipe(filepath)
		if err != nil {
			//TODO: log error
			fmt.Println(err)
			continue
		}

		fmt.Printf("appending %+v", recipe)
		recipes = append(recipes, recipe)
	}
	return recipes, nil
}

func (rm *RecipeManager) loadRecipe(path string) (*Recipe, error) {
	b, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var recipeWrapper recipeJSON
	err = json.Unmarshal(b, &recipeWrapper)
	if err != nil {
		return nil, err
	}

	recipe := &Recipe{}
	recipe.system = rm.System
	recipe.Identifiable = recipeWrapper.Identifiable

	trigger, err := rm.makeTrigger(recipeWrapper.Trigger.Type, recipeWrapper.Trigger.Trigger)
	if err != nil {
		return nil, err
	}
	trigger.SetEnabled(recipeWrapper.Enabled)

	action, err := rm.makeAction(recipeWrapper.Action.Type, recipeWrapper.Action.Action)
	if err != nil {
		return nil, err
	}

	recipe.Trigger = trigger
	recipe.Action = action
	return recipe, nil
}

func (rm *RecipeManager) makeTrigger(triggerID string, triggerIngredients map[string]interface{}) (Trigger, error) {
	trigger := rm.triggerFactory[triggerID]()

	err := setIngredients(trigger, triggerIngredients, reflect.ValueOf(trigger).Elem())
	if err != nil {
		return nil, err
	}
	return trigger, nil
}

func (rm *RecipeManager) makeAction(actionID string, actionIngredients map[string]interface{}) (Action, error) {
	action := rm.actionFactory[actionID]()

	err := setIngredients(action, actionIngredients, reflect.ValueOf(action).Elem())
	if err != nil {
		return nil, err
	}
	return action, nil
}

func setIngredients(i Ingredientor, ingredientValues map[string]interface{}, s reflect.Value) error {
	for _, ingredient := range i.Ingredients() {
		_, ok := ingredientValues[ingredient.ID]

		// This is a required ingredient but the caller did not pass it in
		if !ok && ingredient.Required {
			//TODO: Better error message
			return errors.New("Missing ingredient")
		}

		//TODO: If !ok and !required, set to default value if there is one

		// Value passed in by user matches an ingredient on the trigger, set it
		if ok {
			field := s.FieldByName(ingredient.ID)
			switch ingredient.Type {
			case "string":
				value, ok := ingredientValues[ingredient.ID].(string)
				if !ok {
					return errors.New(fmt.Sprintf("Invalid type, %s must be a string", ingredient.ID))
				}
				field.SetString(value)
			case "boolean":
				value, ok := ingredientValues[ingredient.ID].(bool)
				if !ok {
					return errors.New(fmt.Sprintf("Invalid type, %s must be a boolean", ingredient.ID))
				}
				field.SetBool(value)

			case "integer":
				value, ok := ingredientValues[ingredient.ID].(float64)
				if !ok {
					return errors.New(fmt.Sprintf("Invalid type, %s must be an integer", ingredient.ID))
				}
				field.SetInt(int64(value))

			case "float":
				value, ok := ingredientValues[ingredient.ID].(float64)
				if !ok {
					return errors.New(fmt.Sprintf("Invalid type, %s must be a float", ingredient.ID))
				}
				field.SetFloat(value)

			case "duration":
				value, ok := ingredientValues[ingredient.ID].(float64)
				if !ok {
					return errors.New(fmt.Sprintf("Invalid type, %s must be an integer", ingredient.ID))
				}
				field.Set(reflect.ValueOf(time.Duration(int64(value)) * time.Millisecond))
			case "datetime":
				//TODO: implement

			default:
				return errors.New(fmt.Sprintf("Unknown ingredient type: %s", ingredient.Type))
			}
		}
	}
	return nil
}

func loadCookBooks(dataPath string) []*CookBook {
	// For every cook book we support, add to this list, at some point these can
	// be defined in a config file or in a DB
	cookBooks := []*CookBook{
		{
			Identifiable: Identifiable{
				ID:          "1",
				Name:        "Lutron Smart Bridge Pro",
				Description: "Cook up some goodness for the Lutron Smart Bridge Pro",
			},
			LogoURL: "lutron_400x400.png",
			Triggers: []Trigger{
				// New triggers need to be added to this slice
				&ButtonTrigger{},
				&TimeTrigger{},
			},
			Actions: []Action{
				// New actions need to be added to this slice
				&ZoneSetLevelAction{},
				&ZoneSetLevelToggleAction{},
				&SceneSetAction{},
				&SceneSetToggleAction{},
			},
		},
	}
	return cookBooks
}

func buildTriggerFactory(cookBooks []*CookBook) map[string]func() Trigger {
	factory := make(map[string]func() Trigger)
	for _, cookBook := range cookBooks {
		for _, trigger := range cookBook.Triggers {
			factory[trigger.Type()] = trigger.New
		}
	}
	return factory
}

func buildActionFactory(cookBooks []*CookBook) map[string]func() Action {
	factory := make(map[string]func() Action)
	for _, cookBook := range cookBooks {
		for _, action := range cookBook.Actions {
			factory[action.Type()] = action.New
		}
	}
	return factory
}