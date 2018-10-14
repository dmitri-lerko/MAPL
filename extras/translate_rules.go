package main

import (
	"github.com/octarinesec/MAPL/MAPL_engine"
	"fmt"
	"strings"
	"os"
)

// Package extras contains conversion of rules:
// - rules that are written with conditions on labels are converted to be using only the service names
//

// this is an empty message populated later with message.SenderLabels and message.DestinationLabels to test the rule conditions on the message.
var message_attributes MAPL_engine.MessageAttributes

// The main test calls RemoveLabelConditionsFromRules with the rule structure.
func main() {

	service_labels := make(map[string]map[string]string)

	service_labels["serviceA"]=map[string]string{"key1":"a", "key2":"b", "key3":"c", "key4":"d", "key5":"e","can-access-serviceX":"true"}
	service_labels["serviceB"]=map[string]string{"key1":"a", "key2":"b", "key3":"c", "key4":"d"}
	service_labels["serviceC"]=map[string]string{"key1":"A", "key2":"B", "key3":"C"}
	service_labels["serviceD"]=map[string]string{"key1":"A", "key2":"B","can-access-serviceX":"true"}
	service_labels["serviceE"]=map[string]string{"key1":"abc", "key2":"def","can-access-serviceY":"true"}
	service_labels["serviceX"]=map[string]string{}
	service_labels["serviceY"]=map[string]string{}

	rulesFilename := "examples/rules_for_testing_rule_translation.yaml"
	var rules= MAPL_engine.YamlReadRulesFromFile(rulesFilename)

	messagesFilename := "examples/messages_for_rule_translation.yaml"
	var messages = MAPL_engine.YamlReadMessagesFromFile(messagesFilename)
	message_attributes = messages.Messages[0]

	RemoveLabelConditionsFromRules(rules,service_labels)
}

// function RemoveLabelConditionsFromRules is the main function for translation of conditions on Sender and Receiver Labels to
// the equivalent Sender/Receiver names
func RemoveLabelConditionsFromRules(rules MAPL_engine.Rules, service_labels map[string]map[string]string) (new_rules MAPL_engine.Rules){

	for _, rule := range(rules.Rules){ // go over all of the rules

		translateSenderLabelsFlag:=false
		translateReceiverLabelsFlag:=false

		senders := []string{}
		receivers := []string{}
		receivers_map := map[string][]string{}

		rule_temp:=rule // we work on rule_temp. since we send a pointer to rule_temp and changes are made to it.
		new_rule:=rule // by default we just copy the rule without a change
		// check for label conditions:
		for _,andCondition:=range(rule_temp.DNFConditions){
			for _,condition:=range(andCondition.ANDConditions){
				if condition.AttributeIsSenderLabel{
					//sanity: if we translate sender labels to sender names then the sender name must be "*"
					if rule_temp.Sender!="*"{
						panic("when translating rules: sender!=\"*\"")
					}
					translateSenderLabelsFlag=true
				}
				if condition.AttributeIsReceiverLabel{
					//sanity: if we translate receiver labels to receiver names then the receiver name must be "*"
					if rule_temp.Receiver!="*"{
						panic("when translating rules: receiver!=\"*\"")
					}
					translateReceiverLabelsFlag=true
				}
				if condition.ValueIsReceiverLabel{
					panic("when translating rules: labels as values are not supported")
				}
			}
		}

		if translateSenderLabelsFlag && translateReceiverLabelsFlag{ // this is a special case: there are conditions both on the sender labels and the receiver labels
			rule_temp=rule
			senders = getServicesFromRule(&rule_temp, service_labels, 1) // test which senders satisfy at least one sender label condition
			rule_temp=rule
			receivers = getServicesFromRule(&rule_temp, service_labels, 2)  // test which receivers satisfy at least one receiver label condition
			rule_temp=rule
			receivers_map = getServicesFromRuleWithSenderReceiverLists(&rule_temp, service_labels, senders, receivers) // now go over all the pairs and test which are satisfied together

			useRulePerReceiverFlag:=false // test if all the receivers are satisfied by the same sender list:
			i:=0
			var value0 []string
			for _,value:=range(receivers_map){
				if i==0{
					value0=value
				}else{
					if !testSliceEquality(value,value0){
						useRulePerReceiverFlag=true
					}
				}
				i++
			}

			if !useRulePerReceiverFlag{ // all the receivers are satisfied by the same sender list. then we can make it one rule
				//sanity:
				if !testSliceEquality(senders,value0) {
					panic("somethings is wrong: in \"translateSenderLabelsFlag && translateReceiverLabelsFlag == true\" part")
				}
				new_rule.DNFConditions=removeLabelConditions(&new_rule)
				new_rule.Sender=strings.Join(senders, ";")
				new_rule.Receiver=strings.Join(receivers, ";")
				new_rules.Rules = append(new_rules.Rules, new_rule)
			}else{ // NOT all the receivers are satisfied by the same sender list. then we have to make one rule per receiver in the list
				counter:=0
				for r,s:=range(receivers_map) {
					counter+=1
					new_rule=rule
					new_rule.RuleID=fmt.Sprintf("%s-%d",new_rule.RuleID,counter)
					new_rule.DNFConditions = removeLabelConditions(&new_rule)
					new_rule.Sender = strings.Join(s, ";")
					new_rule.Receiver = r
					new_rules.Rules = append(new_rules.Rules, new_rule)
				}
			}
		}else {
			if translateSenderLabelsFlag { // there are only conditions on the sender labels

				senders = getServicesFromRule(&rule_temp, service_labels, 1)
				if len(senders) == 0 {
					panic("failed to translate sender labels")
				}
				new_rule.DNFConditions=removeLabelConditions(&new_rule)
				new_rule.Sender=strings.Join(senders, ";")
				new_rules.Rules = append(new_rules.Rules, new_rule)
			}
			if translateReceiverLabelsFlag { // there are only conditions on the receiver labels
				receivers = getServicesFromRule(&rule_temp, service_labels, 2)
				if len(receivers) == 0 {
					panic("failed to translate receiver labels")
				}
				new_rule.DNFConditions=removeLabelConditions(&new_rule)
				new_rule.Receiver=strings.Join(receivers, ";")
				new_rules.Rules = append(new_rules.Rules, new_rule)
			}
			if !translateSenderLabelsFlag && !translateReceiverLabelsFlag { // there are no condtions on any labels, so we just copy the rule
				new_rules.Rules = append(new_rules.Rules, new_rule)
			}
		}
	}

	// recreate regular expressions and convert condition values:
	MAPL_engine.ConvertFieldsToRegex(&new_rules)
	new_rules = MAPL_engine.ConvertConditionStringToIntFloatRegex(new_rules)

	filename := "outputs/translated_rules.yaml"
	outputRulesToFile(&new_rules,filename)

	return new_rules
}

// getSendersFromRule translate the conditions on the labels into services that satisfy them
func getServicesFromRule(rule *MAPL_engine.Rule,service_labels map[string]map[string]string,sender_receiver int) (services_list []string){

	for name,labels := range(service_labels) {
		message := message_attributes

		if sender_receiver==1{
			message.SourceLabels = labels
			message.DestinationLabels = nil
		}else{
			message.DestinationLabels = labels
			message.SourceLabels = nil
		}


		rule.DNFConditions = filterLabelRules(rule.DNFConditions,sender_receiver)
		result:=MAPL_engine.TestConditions(rule, &message)

		if result{
			services_list=append(services_list, name)
		}
	}
	return services_list
}

// filterLabelRules: a helper function that removes irrelevant conditions from the rule
func filterLabelRules(dnfConditions []MAPL_engine.ANDConditions,sender_receiver int) (dnfCondtions_out []MAPL_engine.ANDConditions){

	for _, andCondition := range (dnfConditions) {
		andCondition_temp := MAPL_engine.ANDConditions{}
		for _, condition := range (andCondition.ANDConditions) {

			if sender_receiver ==1 {
				if !condition.AttributeIsSenderLabel{
					condition.Attribute = "true" // ignore non-relevant conditions
				}
			}

			if sender_receiver ==2 {
				if !condition.AttributeIsReceiverLabel{
					condition.Attribute = "true" // ignore non-relevant conditions
				}
			}

			if sender_receiver ==3 {
				if  !condition.AttributeIsSenderLabel && !condition.AttributeIsReceiverLabel {
					condition.Attribute = "true" // ignore non-relevant conditions
				}
			}

			andCondition_temp.ANDConditions=append(andCondition_temp.ANDConditions, condition)
		}
		dnfCondtions_out=append(dnfCondtions_out,andCondition_temp)
	}

	return dnfCondtions_out
}


// getServicesFromRuleWithSenderReceiverLists tests the label conditions for all the sender-receiver pairs (given as input)
func getServicesFromRuleWithSenderReceiverLists(rule *MAPL_engine.Rule,service_labels map[string]map[string]string,senders []string,receivers []string) (services_map map[string][]string) {

	services_map = make(map[string][]string)
	for _,r :=range(receivers){
		for _,s:=range (senders){

			if r==s{
				continue
			}

			message := message_attributes
			message.SourceLabels = service_labels[s]
			message.DestinationLabels = service_labels[r]
			// fix rule:
			rule.DNFConditions = filterLabelRules(rule.DNFConditions,3)
			result:=MAPL_engine.TestConditions(rule, &message)
			if result{
				services_map[r]=append(services_map[r],s) // all the senders per receiver
			}
		}
	}
	return services_map
}

// removeLabelConditions: remove conditions with sender/receiver labels. used after we translated the conditions into sender/receiver names
func removeLabelConditions(rule *MAPL_engine.Rule) (new_DNFConditions []MAPL_engine.ANDConditions){
	for _,andCondition:=range(rule.DNFConditions){
		var temp_andConditions MAPL_engine.ANDConditions
		for _,condition:=range(andCondition.ANDConditions) {
			if condition.AttributeIsSenderLabel || condition.AttributeIsReceiverLabel{
				//skip
			}else{
				temp_andConditions.ANDConditions=append(temp_andConditions.ANDConditions,condition)
			}
		}
		if len(temp_andConditions.ANDConditions)>0{
			new_DNFConditions=append(new_DNFConditions,temp_andConditions) // append only if there are still conditions after the removal of the label conditions
		}
	}
	return new_DNFConditions
}

// outputRulesToFile: create a new yaml file of the new translated rules
func outputRulesToFile(rules *MAPL_engine.Rules,filename string){
	f, err := os.Create(filename)
	if err != nil {
		panic(err)
	}
	defer f.Close()

	f.WriteString("rules:\n")
	for _,rule:=range(rules.Rules){
		fmt.Fprintf(f,"\n  - rule_id: %v\n", rule.RuleID)
		fmt.Fprintf(f,"    sender: \"%v\"\n",rule.Sender)
		fmt.Fprintf(f,"    receiver: \"%v\"\n",rule.Receiver)
		fmt.Fprintf(f,"    resource:\n")
		fmt.Fprintf(f,"      resourceProtocol: %v\n",rule.Resource.ResourceProtocol)
		fmt.Fprintf(f,"      resourceType: %v\n",rule.Resource.ResourceType)
		fmt.Fprintf(f,"      resourceName: \"%v\"\n",rule.Resource.ResourceName)
		fmt.Fprintf(f,"    operation: %v\n",rule.Operation)
		if len(rule.DNFConditions)>0 {
			fmt.Fprintf(f, "    DNFconditions:\n")
			for _, and_conditions := range (rule.DNFConditions) {
				fmt.Fprintf(f,"      - ANDConditions:\n")
				for _, condition:=range(and_conditions.ANDConditions) {
					fmt.Fprintf(f, "        - attribute: \"%v\"\n", condition.Attribute)
					fmt.Fprintf(f, "          method: %v\n", condition.Method)
					fmt.Fprintf(f, "          value: \"%v\"\n", condition.Value)
				}
			}
		}
		fmt.Fprintf(f,"    decision: %v\n",rule.Decision)
	}
}
// printDNFConditions: prints the conditions to the console (used for debugging)
func printDNFConditions(dnfConditions []MAPL_engine.ANDConditions) {
	for _,andCondition:=range(dnfConditions) {
		fmt.Println("- ANDCondions:")
		for _, condition := range (andCondition.ANDConditions) {
			fmt.Printf("  - Attribute: %v\n", condition.Attribute)
			fmt.Printf("    Method: %v\n", condition.Method)
			fmt.Printf("    Value: %v\n", condition.Value)
		}
	}
}

// testSliceEquality: compares two slices of strings
func testSliceEquality(a, b []string) bool {

	// If one is nil, the other must also be nil.
	if (a == nil) != (b == nil) {
		return false;
	}

	if len(a) != len(b) {
		return false
	}

	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}

	return true
}