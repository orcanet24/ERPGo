-- ERPGo schema (structure)
-- Generated from SalesERP v12.13 install.sql (InnoDB everywhere, no demo data)
-- Seeds: chart of accounts, predefined accounts, subtypes (accounting backbone)

SET SQL_MODE = "NO_AUTO_VALUE_ON_ZERO";
START TRANSACTION;
SET time_zone = "+00:00";
SET NAMES utf8mb4;

CREATE TABLE IF NOT EXISTS `acc_coa` (
  `id` int NOT NULL,
  `HeadCode` int NOT NULL,
  `HeadName` varchar(100) NOT NULL,
  `PHeadName` varchar(200) NOT NULL,
  `PHeadCode` varchar(50) DEFAULT NULL,
  `HeadLevel` int NOT NULL,
  `IsActive` tinyint(1) NOT NULL,
  `IsTransaction` tinyint(1) NOT NULL,
  `IsGL` tinyint(1) NOT NULL,
  `isCashNature` int NOT NULL DEFAULT '0',
  `isBankNature` int NOT NULL DEFAULT '0',
  `HeadType` char(1) NOT NULL,
  `IsBudget` tinyint(1) NOT NULL,
  `IsDepreciation` tinyint(1) NOT NULL,
  `customer_id` varchar(50) DEFAULT NULL,
  `supplier_id` varchar(50) DEFAULT NULL,
  `bank_id` varchar(100) DEFAULT NULL,
  `service_id` varchar(50) DEFAULT NULL,
  `DepreciationRate` decimal(18,2) NOT NULL,
  `CreateBy` varchar(50) NOT NULL,
  `CreateDate` datetime NOT NULL,
  `UpdateBy` varchar(50) NOT NULL,
  `UpdateDate` datetime NOT NULL,
  `isSubType` int NOT NULL DEFAULT '0',
  `subType` int NOT NULL DEFAULT '1',
  `isStock` int NOT NULL DEFAULT '0',
  `isFixedAssetSch` int NOT NULL DEFAULT '0',
  `noteNo` varchar(20) DEFAULT NULL,
  `assetCode` varchar(20) DEFAULT NULL,
  `depCode` varchar(20) DEFAULT NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- --------------------------------------------------------

--
-- Table structure for table `acc_monthly_balance`
--

CREATE TABLE IF NOT EXISTS `acc_monthly_balance` (
  `id` int NOT NULL,
  `fyear` int NOT NULL,
  `COAID` int NOT NULL,
  `balance1` decimal(18,2) NOT NULL DEFAULT '0.00',
  `balance2` decimal(18,2) NOT NULL DEFAULT '0.00',
  `balance3` decimal(18,2) NOT NULL DEFAULT '0.00',
  `balance4` decimal(18,2) NOT NULL DEFAULT '0.00',
  `balance5` decimal(18,2) NOT NULL DEFAULT '0.00',
  `balance6` decimal(18,2) NOT NULL DEFAULT '0.00',
  `balance7` decimal(18,2) NOT NULL DEFAULT '0.00',
  `balance8` decimal(18,2) NOT NULL DEFAULT '0.00',
  `balance9` decimal(18,2) NOT NULL DEFAULT '0.00',
  `balance10` decimal(18,2) NOT NULL DEFAULT '0.00',
  `balance11` decimal(18,2) NOT NULL DEFAULT '0.00',
  `balance12` decimal(18,2) NOT NULL DEFAULT '0.00',
  `totalBalance` decimal(18,2) NOT NULL DEFAULT '0.00',
  `updatedDate` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- --------------------------------------------------------

--
-- Table structure for table `acc_opening_balance`
--

CREATE TABLE IF NOT EXISTS `acc_opening_balance` (
  `id` int NOT NULL,
  `fyear` int NOT NULL,
  `COAID` int NOT NULL,
  `subType` int NOT NULL DEFAULT '1',
  `subCode` int DEFAULT NULL,
  `Debit` decimal(10,0) NOT NULL,
  `Credit` decimal(10,0) NOT NULL,
  `openDate` date NOT NULL,
  `CreateBy` int NOT NULL,
  `CreateDate` datetime NOT NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- --------------------------------------------------------

--
-- Table structure for table `acc_predefine_account`
--

CREATE TABLE IF NOT EXISTS `acc_predefine_account` (
  `id` int NOT NULL,
  `cashCode` int NOT NULL,
  `bankCode` int NOT NULL,
  `advance` int NOT NULL,
  `fixedAsset` int NOT NULL,
  `purchaseCode` int NOT NULL,
  `salesCode` int NOT NULL,
  `serviceCode` int NOT NULL,
  `customerCode` int NOT NULL,
  `supplierCode` int NOT NULL,
  `costs_of_good_solds` int NOT NULL,
  `vat` int NOT NULL,
  `tax` int NOT NULL,
  `inventoryCode` int NOT NULL,
  `CPLCode` int NOT NULL,
  `LPLCode` int NOT NULL,
  `salary_code` int DEFAULT NULL,
  `emp_npf_contribution` int DEFAULT NULL,
  `empr_npf_contribution` int DEFAULT NULL,
  `emp_icf_contribution` int DEFAULT NULL,
  `empr_icf_contribution` int DEFAULT NULL,
  `prov_state_tax` int NOT NULL,
  `state_tax` int NOT NULL,
  `prov_npfcode` int DEFAULT NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- --------------------------------------------------------

--
-- Table structure for table `acc_subcode`
--

CREATE TABLE IF NOT EXISTS `acc_subcode` (
  `id` int NOT NULL,
  `subTypeId` int NOT NULL,
  `name` varchar(250) CHARACTER SET utf8mb4 COLLATE utf8mb4_general_ci NOT NULL,
  `referenceNo` int NOT NULL,
  `status` int NOT NULL DEFAULT '1',
  `created_by` int NOT NULL,
  `created_date` date NOT NULL,
  `updated_by` int NOT NULL,
  `updated_date` timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- --------------------------------------------------------

--
-- Table structure for table `acc_subtype`
--

CREATE TABLE IF NOT EXISTS `acc_subtype` (
  `id` int NOT NULL,
  `subtypeName` varchar(200) CHARACTER SET utf8mb4 COLLATE utf8mb4_general_ci NOT NULL,
  `staus` int NOT NULL DEFAULT '1',
  `created_by` int NOT NULL,
  `created_date` datetime NOT NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- --------------------------------------------------------

--
-- Table structure for table `acc_transaction`
--

CREATE TABLE IF NOT EXISTS `acc_transaction` (
  `ID` int NOT NULL,
  `vid` int NOT NULL,
  `fyear` int NOT NULL,
  `VNo` varchar(50) CHARACTER SET utf8mb3 COLLATE utf8mb3_general_ci DEFAULT NULL,
  `Vtype` varchar(50) CHARACTER SET utf8mb3 COLLATE utf8mb3_general_ci DEFAULT NULL,
  `referenceNo` varchar(20) CHARACTER SET utf8mb3 COLLATE utf8mb3_unicode_ci DEFAULT NULL,
  `VDate` date DEFAULT NULL,
  `COAID` varchar(50) CHARACTER SET utf8mb3 COLLATE utf8mb3_general_ci NOT NULL,
  `Narration` text CHARACTER SET utf8mb3 COLLATE utf8mb3_general_ci,
  `chequeNo` varchar(30) CHARACTER SET utf8mb3 COLLATE utf8mb3_unicode_ci DEFAULT NULL,
  `chequeDate` date DEFAULT NULL,
  `isHonour` int DEFAULT NULL,
  `ledgerComment` varchar(100) CHARACTER SET utf8mb3 COLLATE utf8mb3_unicode_ci NOT NULL DEFAULT '0',
  `Debit` decimal(18,2) DEFAULT NULL,
  `Credit` decimal(18,2) DEFAULT NULL,
  `StoreID` int NOT NULL DEFAULT '0',
  `IsPosted` char(10) CHARACTER SET utf8mb3 COLLATE utf8mb3_general_ci DEFAULT NULL,
  `is_opening` int NOT NULL DEFAULT '0',
  `CreateBy` varchar(50) CHARACTER SET utf8mb3 COLLATE utf8mb3_general_ci DEFAULT NULL,
  `CreateDate` datetime DEFAULT NULL,
  `UpdateBy` varchar(50) CHARACTER SET utf8mb3 COLLATE utf8mb3_general_ci DEFAULT NULL,
  `UpdateDate` datetime DEFAULT NULL,
  `IsAppove` char(10) CHARACTER SET utf8mb3 COLLATE utf8mb3_general_ci DEFAULT NULL,
  `RevCodde` bigint NOT NULL,
  `subType` int NOT NULL DEFAULT '1',
  `subCode` int DEFAULT NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- --------------------------------------------------------

--
-- Table structure for table `acc_vaucher`
--

CREATE TABLE IF NOT EXISTS `acc_vaucher` (
  `id` int NOT NULL,
  `fyear` int NOT NULL,
  `VNo` varchar(50) CHARACTER SET utf8mb4 COLLATE utf8mb4_general_ci NOT NULL,
  `Vtype` varchar(50) CHARACTER SET utf8mb4 COLLATE utf8mb4_general_ci NOT NULL,
  `referenceNo` varchar(50) CHARACTER SET utf8mb4 COLLATE utf8mb4_general_ci DEFAULT NULL,
  `VDate` date NOT NULL,
  `COAID` int NOT NULL,
  `Narration` varchar(255) CHARACTER SET utf8mb4 COLLATE utf8mb4_general_ci DEFAULT NULL,
  `chequeno` varchar(30) CHARACTER SET utf8mb4 COLLATE utf8mb4_general_ci DEFAULT NULL,
  `chequeDate` date DEFAULT NULL,
  `isHonour` int NOT NULL DEFAULT '0',
  `ledgerComment` varchar(255) CHARACTER SET utf8mb4 COLLATE utf8mb4_general_ci DEFAULT NULL,
  `Debit` decimal(18,2) NOT NULL DEFAULT '0.00',
  `Credit` decimal(18,2) NOT NULL DEFAULT '0.00',
  `RevCodde` int NOT NULL,
  `subType` int NOT NULL DEFAULT '1',
  `subCode` int DEFAULT NULL,
  `isApproved` int NOT NULL DEFAULT '0',
  `CreateBy` int NOT NULL,
  `CreateDate` datetime NOT NULL,
  `UpdateBy` int DEFAULT NULL,
  `UpdateDate` datetime DEFAULT NULL,
  `approvedBy` int DEFAULT NULL,
  `approvedDate` datetime DEFAULT NULL,
  `isyearClosed` tinyint(1) NOT NULL DEFAULT '0',
  `status` int NOT NULL DEFAULT '0' COMMENT '0 = non yet transfer to transation,  1 = Tranfer to transition'
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- --------------------------------------------------------

--
-- Table structure for table `activity_logs`
--

CREATE TABLE IF NOT EXISTS `activity_logs` (
  `activity_id` int NOT NULL,
  `user_id` varchar(11) CHARACTER SET utf8mb4 COLLATE utf8mb4_general_ci NOT NULL COMMENT 'for employee , it will save employee_id',
  `type` varchar(30) CHARACTER SET utf8mb4 COLLATE utf8mb4_general_ci NOT NULL COMMENT 'ticket, employee, attendnace etc.',
  `action` varchar(15) CHARACTER SET utf8mb4 COLLATE utf8mb4_general_ci NOT NULL COMMENT 'create, update, delete',
  `action_id` varchar(15) CHARACTER SET utf8mb4 COLLATE utf8mb4_general_ci NOT NULL,
  `table_name` varchar(30) CHARACTER SET utf8mb4 COLLATE utf8mb4_general_ci DEFAULT NULL,
  `slug` varchar(150) CHARACTER SET utf8mb4 COLLATE utf8mb4_general_ci NOT NULL,
  `form_data` text CHARACTER SET utf8mb4 COLLATE utf8mb4_general_ci,
  `create_date` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `status` tinyint(1) NOT NULL DEFAULT '0' COMMENT '1=created, 2=updated, 3=deleted	'
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- --------------------------------------------------------

--
-- Table structure for table `ai_report_types`
--

CREATE TABLE IF NOT EXISTS `ai_report_types` (
  `id` int NOT NULL,
  `name` varchar(255) NOT NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- --------------------------------------------------------

--
-- Table structure for table `ai_setting`
--

CREATE TABLE IF NOT EXISTS `ai_setting` (
  `setting_id` int NOT NULL,
  `api_key` varchar(250) DEFAULT NULL,
  `model` varchar(50) CHARACTER SET utf8mb3 COLLATE utf8mb3_general_ci DEFAULT '1',
  `temperature` float DEFAULT '0.7',
  `max_tokens` int DEFAULT '400',
  `prompt_template` text,
  `language` varchar(255) DEFAULT NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- --------------------------------------------------------

--
-- Table structure for table `app_setting`
--

CREATE TABLE IF NOT EXISTS `app_setting` (
  `id` int NOT NULL,
  `localhserver` varchar(250) DEFAULT NULL,
  `onlineserver` varchar(250) DEFAULT NULL,
  `hotspot` varchar(250) DEFAULT NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- --------------------------------------------------------

--
-- Table structure for table `attendance`
--

CREATE TABLE IF NOT EXISTS `attendance` (
  `att_id` int NOT NULL,
  `employee_id` int NOT NULL,
  `date` date NOT NULL,
  `sign_in` varchar(30) NOT NULL,
  `sign_out` varchar(30) NOT NULL,
  `staytime` varchar(30) NOT NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- --------------------------------------------------------

--
-- Table structure for table `bank_add`
--

CREATE TABLE IF NOT EXISTS `bank_add` (
  `id` int NOT NULL,
  `bank_id` varchar(255) NOT NULL,
  `bank_name` varchar(255) NOT NULL,
  `ac_name` varchar(250) DEFAULT NULL,
  `ac_number` varchar(250) DEFAULT NULL,
  `branch` varchar(250) DEFAULT NULL,
  `signature_pic` varchar(250) DEFAULT NULL,
  `status` int DEFAULT NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- --------------------------------------------------------

--
-- Table structure for table `closing_records`
--

CREATE TABLE IF NOT EXISTS `closing_records` (
  `id` int NOT NULL,
  `head_code` bigint DEFAULT NULL,
  `opening_balance` decimal(10,2) NOT NULL DEFAULT '0.00',
  `amount_in` decimal(10,2) NOT NULL DEFAULT '0.00',
  `amount_out` decimal(10,2) NOT NULL DEFAULT '0.00',
  `closign_balance` decimal(10,2) NOT NULL DEFAULT '0.00',
  `datetime` timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `user_id` varchar(20) CHARACTER SET utf8mb4 COLLATE utf8mb4_general_ci DEFAULT NULL,
  `status` int DEFAULT NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- --------------------------------------------------------

--
-- Table structure for table `company_information`
--

CREATE TABLE IF NOT EXISTS `company_information` (
  `company_id` varchar(250) NOT NULL,
  `company_name` varchar(50) NOT NULL,
  `email` varchar(50) NOT NULL,
  `address` text NOT NULL,
  `mobile` varchar(50) NOT NULL,
  `website` varchar(50) NOT NULL,
  `vat_no` varchar(50) DEFAULT NULL,
  `cr_no` varchar(50) DEFAULT NULL,
  `upload_path` varchar(64) DEFAULT NULL COMMENT 'Directorio privado de archivos (nombre aleatorio)',
  `status` int NOT NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- --------------------------------------------------------

--
-- Table structure for table `currency_tbl`
--

CREATE TABLE IF NOT EXISTS `currency_tbl` (
  `id` int NOT NULL,
  `currency_name` varchar(50) NOT NULL,
  `icon` text NOT NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- --------------------------------------------------------

--
-- Table structure for table `customer_information`
--

CREATE TABLE IF NOT EXISTS `customer_information` (
  `customer_id` bigint NOT NULL,
  `customer_name` varchar(255) DEFAULT NULL,
  `customer_address` varchar(255) DEFAULT NULL,
  `address2` text NOT NULL,
  `customer_mobile` varchar(100) DEFAULT NULL,
  `customer_email` varchar(100) DEFAULT NULL,
  `email_address` varchar(200) DEFAULT NULL,
  `contact` varchar(100) DEFAULT NULL,
  `phone` varchar(50) DEFAULT NULL,
  `fax` varchar(100) DEFAULT NULL,
  `city` text,
  `state` text,
  `zip` varchar(50) DEFAULT NULL,
  `country` varchar(250) DEFAULT NULL,
  `status` int NOT NULL COMMENT '1=paid,2=credit',
  `create_date` timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `create_by` varchar(30) DEFAULT NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- --------------------------------------------------------

--
-- Table structure for table `daily_banking_add`
--

CREATE TABLE IF NOT EXISTS `daily_banking_add` (
  `banking_id` varchar(255) NOT NULL,
  `date` datetime DEFAULT NULL,
  `bank_id` varchar(100) DEFAULT NULL,
  `deposit_type` varchar(255) DEFAULT NULL,
  `transaction_type` varchar(255) DEFAULT NULL,
  `description` text,
  `amount` int DEFAULT NULL,
  `status` int DEFAULT NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- --------------------------------------------------------

--
-- Table structure for table `daily_closing`
--

CREATE TABLE IF NOT EXISTS `daily_closing` (
  `closing_id` int NOT NULL,
  `last_day_closing` float NOT NULL,
  `cash_in` float NOT NULL,
  `cash_out` float NOT NULL,
  `date` varchar(250) NOT NULL,
  `amount` float NOT NULL,
  `adjustment` float DEFAULT NULL,
  `status` int NOT NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- --------------------------------------------------------

--
-- Table structure for table `designation`
--

CREATE TABLE IF NOT EXISTS `designation` (
  `id` int NOT NULL,
  `designation` varchar(150) NOT NULL,
  `details` text NOT NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- --------------------------------------------------------

--
-- Table structure for table `email_config`
--

CREATE TABLE IF NOT EXISTS `email_config` (
  `id` int NOT NULL,
  `protocol` text NOT NULL,
  `smtp_host` text NOT NULL,
  `smtp_port` text NOT NULL,
  `smtp_user` varchar(35) NOT NULL,
  `smtp_pass` varchar(35) NOT NULL,
  `mailtype` varchar(30) DEFAULT NULL,
  `isinvoice` tinyint NOT NULL,
  `isservice` tinyint NOT NULL,
  `isquotation` tinyint NOT NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- --------------------------------------------------------

--
-- Table structure for table `employee_history`
--

CREATE TABLE IF NOT EXISTS `employee_history` (
  `id` int NOT NULL,
  `first_name` varchar(50) NOT NULL,
  `last_name` varchar(50) NOT NULL,
  `designation` varchar(100) NOT NULL,
  `phone` varchar(50) NOT NULL,
  `rate_type` int NOT NULL,
  `hrate` float NOT NULL,
  `email` varchar(50) NOT NULL,
  `blood_group` varchar(10) NOT NULL,
  `address_line_1` text NOT NULL,
  `address_line_2` text NOT NULL,
  `image` text,
  `country` varchar(50) NOT NULL,
  `city` varchar(50) NOT NULL,
  `zip` varchar(50) NOT NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- --------------------------------------------------------

--
-- Table structure for table `employee_salary_payment`
--

CREATE TABLE IF NOT EXISTS `employee_salary_payment` (
  `emp_sal_pay_id` int NOT NULL,
  `generate_id` int NOT NULL,
  `employee_id` varchar(50) CHARACTER SET latin1 COLLATE latin1_swedish_ci NOT NULL,
  `total_salary` decimal(18,2) NOT NULL DEFAULT '0.00',
  `total_working_minutes` varchar(50) CHARACTER SET latin1 COLLATE latin1_swedish_ci NOT NULL,
  `working_period` varchar(50) CHARACTER SET latin1 COLLATE latin1_swedish_ci NOT NULL,
  `payment_due` varchar(50) CHARACTER SET latin1 COLLATE latin1_swedish_ci NOT NULL,
  `payment_date` varchar(50) CHARACTER SET latin1 COLLATE latin1_swedish_ci NOT NULL,
  `salary_name` varchar(100) DEFAULT NULL,
  `payment_type` int DEFAULT NULL,
  `bank_name` varchar(255) DEFAULT NULL,
  `paid_by` varchar(50) CHARACTER SET latin1 COLLATE latin1_swedish_ci NOT NULL,
  `salary_month` varchar(70) DEFAULT NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- --------------------------------------------------------

--
-- Table structure for table `employee_salary_setup`
--

CREATE TABLE IF NOT EXISTS `employee_salary_setup` (
  `e_s_s_id` int UNSIGNED NOT NULL,
  `employee_id` varchar(30) CHARACTER SET latin1 COLLATE latin1_swedish_ci NOT NULL,
  `sal_type` varchar(30) NOT NULL,
  `salary_type_id` varchar(30) CHARACTER SET latin1 COLLATE latin1_swedish_ci NOT NULL,
  `amount` decimal(12,2) NOT NULL DEFAULT '0.00',
  `is_percentage` int DEFAULT NULL COMMENT 'all amount will add or deduct on percent if true ',
  `create_date` date DEFAULT NULL,
  `update_date` datetime(6) DEFAULT NULL,
  `update_id` varchar(30) CHARACTER SET latin1 COLLATE latin1_swedish_ci NOT NULL,
  `gross_salary` float NOT NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- --------------------------------------------------------

--
-- Table structure for table `expense`
--

CREATE TABLE IF NOT EXISTS `expense` (
  `id` int NOT NULL,
  `date` date NOT NULL,
  `type` varchar(100) NOT NULL,
  `voucher_no` varchar(50) NOT NULL,
  `amount` float NOT NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- --------------------------------------------------------

--
-- Table structure for table `expense_item`
--

CREATE TABLE IF NOT EXISTS `expense_item` (
  `id` int NOT NULL,
  `expense_item_name` varchar(200) NOT NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- --------------------------------------------------------

--
-- Table structure for table `financial_year`
--

CREATE TABLE IF NOT EXISTS `financial_year` (
  `id` int NOT NULL,
  `yearName` varchar(30) NOT NULL,
  `startDate` date NOT NULL,
  `endDate` date NOT NULL,
  `isCloseReq` tinyint(1) NOT NULL DEFAULT '0',
  `status` tinyint(1) NOT NULL DEFAULT '2' COMMENT '1=active, 0=closed, 2=pending',
  `created_by` int NOT NULL,
  `created_date` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `updated_by` int DEFAULT NULL,
  `updated_date` datetime DEFAULT NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- --------------------------------------------------------

--
-- Table structure for table `gmb_salary_advance`
--

CREATE TABLE IF NOT EXISTS `gmb_salary_advance` (
  `id` int NOT NULL,
  `employee_id` int NOT NULL,
  `salary_month` varchar(50) CHARACTER SET utf8mb4 COLLATE utf8mb4_general_ci NOT NULL COMMENT 'for the month advance will be deducted',
  `amount` decimal(11,0) NOT NULL,
  `release_amount` decimal(11,0) DEFAULT '0',
  `paid` tinyint NOT NULL DEFAULT '0' COMMENT 'paid_to_employee',
  `CreateDate` date NOT NULL,
  `CreateBy` int NOT NULL,
  `UpdateDate` date DEFAULT NULL,
  `UpdateBy` int DEFAULT NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- --------------------------------------------------------

--
-- Table structure for table `gmb_salary_generate`
--

CREATE TABLE IF NOT EXISTS `gmb_salary_generate` (
  `id` int NOT NULL,
  `sal_month_year` varchar(50) CHARACTER SET utf8mb4 COLLATE utf8mb4_general_ci NOT NULL,
  `employee_id` varchar(11) CHARACTER SET utf8mb4 COLLATE utf8mb4_general_ci NOT NULL,
  `tin_no` int DEFAULT NULL COMMENT 'TIN No',
  `total_attendance` varchar(11) CHARACTER SET utf8mb4 COLLATE utf8mb4_general_ci DEFAULT NULL COMMENT 'tagret_hours / days',
  `total_count` varchar(11) CHARACTER SET utf8mb4 COLLATE utf8mb4_general_ci DEFAULT NULL COMMENT 'weorked_hours / days',
  `atten_allowance` decimal(11,2) DEFAULT NULL COMMENT 'based on taget hours / days',
  `gross` decimal(11,2) NOT NULL COMMENT 'from employee_file table',
  `basic` decimal(11,2) NOT NULL COMMENT 'from employee_file table',
  `transport` decimal(11,2) NOT NULL,
  `house_rent` decimal(11,2) DEFAULT NULL,
  `medical` decimal(11,2) DEFAULT NULL,
  `other_allowance` decimal(11,2) DEFAULT NULL,
  `gross_salary` decimal(11,2) NOT NULL COMMENT 'after adding all allowance with basic',
  `income_tax` decimal(11,2) DEFAULT NULL COMMENT 'from employee_file table it will convert to amount',
  `soc_sec_npf_tax` decimal(11,2) DEFAULT NULL COMMENT 'from employee_file table it will convert to amount',
  `employer_contribution` decimal(11,2) DEFAULT NULL COMMENT '10 % of basic if there soc.sec.tax available ',
  `icf_amount` decimal(11,0) NOT NULL COMMENT 'Id social security tax available',
  `loan_deduct` decimal(11,2) DEFAULT NULL,
  `loan_id` int DEFAULT NULL COMMENT 'From grand_loan table',
  `salary_advance` decimal(11,2) DEFAULT NULL,
  `salary_adv_id` int DEFAULT NULL COMMENT 'From gmb_salary_advance table',
  `lwp` decimal(11,2) DEFAULT NULL COMMENT 'leave without pay	',
  `pf` decimal(11,2) DEFAULT NULL COMMENT 'providend fund',
  `stamp` decimal(11,2) DEFAULT NULL COMMENT 'deduct type',
  `net_salary` decimal(11,2) DEFAULT NULL COMMENT 'after deduct net amount from gross salary',
  `createDate` date DEFAULT NULL,
  `createBy` varchar(11) CHARACTER SET utf8mb4 COLLATE utf8mb4_general_ci NOT NULL,
  `updateDate` date DEFAULT NULL,
  `updateBy` varchar(11) CHARACTER SET utf8mb4 COLLATE utf8mb4_general_ci DEFAULT NULL,
  `medical_benefit` decimal(11,2) DEFAULT NULL,
  `family_benefit` decimal(11,2) DEFAULT NULL,
  `transportation_benefit` decimal(11,2) DEFAULT NULL,
  `other_benefit` decimal(11,2) DEFAULT NULL,
  `normal_working_hrs_month` decimal(20,2) DEFAULT NULL,
  `actual_working_hrs_month` decimal(20,2) DEFAULT NULL,
  `hourly_rate_basic` decimal(20,2) DEFAULT NULL,
  `hourly_rate_trasport_allowance` decimal(20,2) DEFAULT NULL,
  `basic_salary_pro_rated` decimal(20,2) DEFAULT NULL,
  `transport_allowance_pro_rated` decimal(20,2) DEFAULT NULL,
  `basic_transport_allowance` decimal(20,2) DEFAULT NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- --------------------------------------------------------

--
-- Table structure for table `gmb_salary_sheet_generate`
--

CREATE TABLE IF NOT EXISTS `gmb_salary_sheet_generate` (
  `ssg_id` int NOT NULL,
  `name` varchar(30) CHARACTER SET utf8mb4 COLLATE utf8mb4_general_ci NOT NULL,
  `gdate` varchar(30) CHARACTER SET utf8mb4 COLLATE utf8mb4_general_ci DEFAULT NULL,
  `start_date` varchar(30) CHARACTER SET utf8mb4 COLLATE utf8mb4_general_ci NOT NULL,
  `end_date` varchar(30) CHARACTER SET utf8mb4 COLLATE utf8mb4_general_ci NOT NULL,
  `generate_by` varchar(30) CHARACTER SET utf8mb4 COLLATE utf8mb4_general_ci NOT NULL COMMENT 'user_id',
  `approved` tinyint(1) DEFAULT '0' COMMENT '1 = approved, 0= not approved',
  `approved_by` varchar(20) CHARACTER SET utf8mb4 COLLATE utf8mb4_general_ci DEFAULT NULL,
  `approved_date` date DEFAULT NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- --------------------------------------------------------

--
-- Table structure for table `invoice`
--

CREATE TABLE IF NOT EXISTS `invoice` (
  `id` int NOT NULL,
  `invoice_id` bigint NOT NULL,
  `customer_id` bigint NOT NULL,
  `date` varchar(50) DEFAULT NULL,
  `cash_amount` decimal(10,2) NOT NULL DEFAULT '0.00',
  `card_amount` decimal(10,2) NOT NULL DEFAULT '0.00',
  `total_amount` decimal(18,2) NOT NULL DEFAULT '0.00',
  `paid_amount` decimal(10,2) NOT NULL DEFAULT '0.00',
  `due_amount` decimal(10,2) DEFAULT '0.00',
  `prevous_due` decimal(20,2) NOT NULL DEFAULT '0.00',
  `shipping_cost` decimal(10,2) NOT NULL DEFAULT '0.00',
  `invoice` bigint NOT NULL,
  `offline_invoice_no` bigint DEFAULT NULL,
  `invoice_discount` decimal(10,2) DEFAULT '0.00' COMMENT 'invoice discount',
  `total_discount` decimal(10,2) DEFAULT '0.00' COMMENT 'total invoice discount',
  `total_vat_amnt` decimal(10,2) NOT NULL DEFAULT '0.00' COMMENT 'total invoice vat',
  `total_tax` decimal(10,2) DEFAULT '0.00',
  `ret_adjust_amnt` decimal(10,2) DEFAULT NULL,
  `returnable_amount` decimal(10,2) DEFAULT NULL,
  `sales_by` varchar(50) NOT NULL,
  `delivery_note` text,
  `invoice_details` text NOT NULL,
  `is_credit` tinyint DEFAULT NULL,
  `is_fixed` int NOT NULL DEFAULT '0',
  `is_draft` varchar(50) NOT NULL DEFAULT 'no',
  `is_dynamic` int NOT NULL DEFAULT '0',
  `status` int NOT NULL,
  `bank_id` varchar(30) DEFAULT NULL,
  `payment_type` int NOT NULL,
  `is_online` int NOT NULL DEFAULT '1'
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- --------------------------------------------------------

--
-- Table structure for table `invoice_details`
--

CREATE TABLE IF NOT EXISTS `invoice_details` (
  `id` int NOT NULL,
  `invoice_details_id` varchar(100) NOT NULL,
  `invoice_id` varchar(100) NOT NULL,
  `ret_invoice_id` bigint DEFAULT NULL,
  `product_id` varchar(30) NOT NULL,
  `serial_no` varchar(30) DEFAULT NULL,
  `description` varchar(250) DEFAULT NULL,
  `quantity` decimal(10,2) DEFAULT NULL,
  `rate` decimal(10,2) DEFAULT NULL,
  `batch_id` varchar(30) DEFAULT NULL,
  `supplier_rate` float DEFAULT NULL,
  `total_price` decimal(12,2) DEFAULT NULL,
  `discount` decimal(10,2) DEFAULT NULL,
  `discount_per` varchar(15) DEFAULT NULL,
  `vat_amnt` decimal(10,2) DEFAULT NULL,
  `vat_amnt_per` varchar(15) DEFAULT NULL,
  `tax` decimal(10,2) DEFAULT NULL,
  `paid_amount` decimal(12,0) DEFAULT NULL,
  `due_amount` decimal(10,2) DEFAULT NULL,
  `status` int NOT NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- --------------------------------------------------------

--
-- Table structure for table `language`
--

CREATE TABLE IF NOT EXISTS `language` (
  `id` int UNSIGNED NOT NULL,
  `phrase` text NOT NULL,
  `english` text
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- --------------------------------------------------------

--
-- Table structure for table `module`
--

CREATE TABLE IF NOT EXISTS `module` (
  `id` int NOT NULL,
  `name` varchar(50) NOT NULL,
  `description` text,
  `image` varchar(255) DEFAULT NULL,
  `directory` varchar(100) DEFAULT NULL,
  `status` tinyint(1) NOT NULL DEFAULT '1'
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- --------------------------------------------------------
--
-- Table structure for table `menu`
--
-- Sidebar dinámico: cada fila es una entrada del menú lateral.
--

CREATE TABLE IF NOT EXISTS `web_sessions` (
  `id` VARCHAR(191) PRIMARY KEY,
  `user_id` VARCHAR(64) NOT NULL DEFAULT '',
  `username` VARCHAR(128) NOT NULL DEFAULT '',
  `fullname` VARCHAR(191) NOT NULL DEFAULT '',
  `user_level` VARCHAR(64) NOT NULL DEFAULT '',
  `image` VARCHAR(255) NOT NULL DEFAULT '',
  `tenant_code` VARCHAR(64) NOT NULL DEFAULT '',
  `is_agent` INTEGER NOT NULL DEFAULT 0,
  `csrf_token` VARCHAR(191) NOT NULL DEFAULT '',
  `expires_at` VARCHAR(32) NOT NULL DEFAULT '',
  `created_at` VARCHAR(32) NOT NULL DEFAULT ''
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS `menu` (
  `id`            int          NOT NULL AUTO_INCREMENT,  -- AUTO_INCREMENT: los addons siembran sus entradas de menú SIN id (addons.go) y MySQL exige que la primaria la genere. La siembra de schema_z_menu.sql sigue pasando ids fijos, que AUTO_INCREMENT admite sin problema.
  `parent_id`     int          DEFAULT NULL,
  `label`         varchar(128) NOT NULL,
  `url`           varchar(255) DEFAULT NULL,
  `icon`          varchar(80)  DEFAULT NULL,
  `perm`          varchar(120) DEFAULT NULL,
  `active_prefix` varchar(120) DEFAULT NULL,
  `sort_order`    int          NOT NULL DEFAULT 0,
  `status`        tinyint(1)   NOT NULL DEFAULT 1,
  `module_id`     varchar(64)  NOT NULL DEFAULT 'core',
  `is_core`       tinyint(1)   NOT NULL DEFAULT 0,
  PRIMARY KEY (`id`),
  KEY `idx_menu_parent` (`parent_id`, `sort_order`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS `tenants` (
  `code` VARCHAR(64) PRIMARY KEY,
  `legal_name` VARCHAR(255) NOT NULL DEFAULT '',
  `rif` VARCHAR(32) NOT NULL DEFAULT '',
  `status` INTEGER NOT NULL DEFAULT 1,
  `created_at` VARCHAR(32) NOT NULL DEFAULT '',
  `db_user` VARCHAR(128) NOT NULL DEFAULT '',
  `db_password_enc` VARBINARY(512) DEFAULT '',
  `max_users` INTEGER NOT NULL DEFAULT 0,
  `created_by` VARCHAR(128) NOT NULL DEFAULT '',
  `db_size_mb` INTEGER NOT NULL DEFAULT 0
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS `tenant_users` (
  `tenant_code` VARCHAR(64) NOT NULL,
  `username` VARCHAR(128) NOT NULL,
  `role_id` INTEGER NOT NULL DEFAULT 0,
  `is_owner` INTEGER NOT NULL DEFAULT 0,
  `status` INTEGER NOT NULL DEFAULT 1,
  `created_at` VARCHAR(32) NOT NULL DEFAULT '',
  PRIMARY KEY (`tenant_code`, `username`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS `tenant_provision_log` (
  `id` INT NOT NULL AUTO_INCREMENT PRIMARY KEY,
  `tenant_code` VARCHAR(64) NOT NULL,
  `action` VARCHAR(64) NOT NULL,
  `detail` VARCHAR(500) NOT NULL DEFAULT '',
  `created_at` VARCHAR(32) NOT NULL DEFAULT ''
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- --------------------------------------------------------

--
-- Table structure for table `module_purchase_key`
--

CREATE TABLE IF NOT EXISTS `module_purchase_key` (
  `id` int NOT NULL,
  `identity` varchar(100) CHARACTER SET utf8mb4 COLLATE utf8mb4_general_ci DEFAULT NULL,
  `purchase_key` varchar(100) CHARACTER SET utf8mb4 COLLATE utf8mb4_general_ci DEFAULT NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- --------------------------------------------------------

--
-- Table structure for table `payroll_tax_setup`
--

CREATE TABLE IF NOT EXISTS `payroll_tax_setup` (
  `tax_setup_id` int UNSIGNED NOT NULL,
  `start_amount` decimal(12,2) NOT NULL DEFAULT '0.00',
  `end_amount` decimal(12,2) NOT NULL DEFAULT '0.00',
  `rate` decimal(12,2) NOT NULL DEFAULT '0.00',
  `status` varchar(30) CHARACTER SET latin1 COLLATE latin1_swedish_ci NOT NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- --------------------------------------------------------

--
-- Table structure for table `personal_loan`
--

CREATE TABLE IF NOT EXISTS `personal_loan` (
  `per_loan_id` int NOT NULL,
  `transaction_id` varchar(30) NOT NULL,
  `person_id` varchar(30) NOT NULL,
  `debit` decimal(12,2) DEFAULT '0.00',
  `credit` decimal(12,2) NOT NULL DEFAULT '0.00',
  `date` varchar(30) NOT NULL,
  `details` varchar(100) NOT NULL,
  `status` int NOT NULL COMMENT '1=no paid,2=paid'
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- --------------------------------------------------------

--
-- Table structure for table `person_information`
--

CREATE TABLE IF NOT EXISTS `person_information` (
  `id` int NOT NULL,
  `person_id` varchar(50) NOT NULL,
  `person_name` varchar(50) NOT NULL,
  `person_phone` varchar(50) NOT NULL,
  `person_address` text NOT NULL,
  `status` int NOT NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- --------------------------------------------------------

--
-- Table structure for table `person_ledger`
--

CREATE TABLE IF NOT EXISTS `person_ledger` (
  `id` int NOT NULL,
  `transaction_id` varchar(50) NOT NULL,
  `person_id` varchar(50) NOT NULL,
  `date` varchar(50) NOT NULL,
  `debit` decimal(12,2) NOT NULL DEFAULT '0.00',
  `credit` decimal(12,2) NOT NULL DEFAULT '0.00',
  `details` text NOT NULL,
  `status` int NOT NULL COMMENT '1=no paid,2=paid'
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- --------------------------------------------------------

--
-- Table structure for table `pesonal_loan_information`
--

CREATE TABLE IF NOT EXISTS `pesonal_loan_information` (
  `id` int NOT NULL,
  `person_id` varchar(50) NOT NULL,
  `person_name` varchar(50) NOT NULL,
  `person_phone` varchar(30) NOT NULL,
  `person_address` text NOT NULL,
  `status` int NOT NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- --------------------------------------------------------

--
-- Table structure for table `print_setting`
--

CREATE TABLE IF NOT EXISTS `print_setting` (
  `id` int NOT NULL,
  `header` int NOT NULL,
  `footer` int NOT NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- --------------------------------------------------------

--
-- Table structure for table `product_category`
--

CREATE TABLE IF NOT EXISTS `product_category` (
  `category_id` int NOT NULL,
  `category_name` varchar(255) DEFAULT NULL,
  `status` int NOT NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- --------------------------------------------------------

--
-- Table structure for table `product_information`
--

CREATE TABLE IF NOT EXISTS `product_information` (
  `id` int NOT NULL,
  `product_id` varchar(100) NOT NULL,
  `category_id` varchar(255) DEFAULT NULL,
  `product_name` varchar(255) NOT NULL,
  `price` varchar(100) DEFAULT NULL,
  `unit` varchar(50) DEFAULT NULL,
  `tax` float DEFAULT NULL COMMENT 'Tax in %',
  `serial_no` varchar(200) DEFAULT NULL,
  `product_vat` float DEFAULT NULL,
  `product_model` varchar(100) DEFAULT NULL,
  `product_details` varchar(255) DEFAULT NULL,
  `image` varchar(255) DEFAULT NULL,
  `status` int DEFAULT NULL,
  `tax0` text,
  `tax1` text
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- --------------------------------------------------------

--
-- Table structure for table `product_purchase`
--

CREATE TABLE IF NOT EXISTS `product_purchase` (
  `id` int NOT NULL,
  `purchase_id` bigint NOT NULL,
  `chalan_no` varchar(100) NOT NULL,
  `supplier_id` bigint NOT NULL,
  `grand_total_amount` decimal(12,2) NOT NULL DEFAULT '0.00',
  `paid_amount` decimal(10,2) DEFAULT '0.00',
  `due_amount` decimal(10,2) DEFAULT '0.00',
  `total_discount` decimal(10,2) DEFAULT NULL,
  `invoice_discount` decimal(10,2) NOT NULL DEFAULT '0.00' COMMENT 'sum of product discount',
  `total_vat_amnt` decimal(10,2) NOT NULL DEFAULT '0.00' COMMENT 'sum of product vat',
  `purchase_date` varchar(50) DEFAULT NULL,
  `purchase_details` text,
  `status` int NOT NULL,
  `bank_id` varchar(30) DEFAULT NULL,
  `payment_type` int NOT NULL,
  `is_credit` tinyint DEFAULT NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- --------------------------------------------------------

--
-- Table structure for table `product_purchase_details`
--

CREATE TABLE IF NOT EXISTS `product_purchase_details` (
  `id` int NOT NULL,
  `purchase_detail_id` varchar(100) DEFAULT NULL,
  `purchase_id` bigint DEFAULT NULL,
  `product_id` varchar(30) DEFAULT NULL,
  `quantity` decimal(10,2) DEFAULT NULL,
  `rate` decimal(10,2) DEFAULT NULL,
  `batch_id` varchar(30) DEFAULT NULL,
  `expiry_date` date DEFAULT NULL,
  `total_amount` decimal(10,2) DEFAULT NULL,
  `discount` float DEFAULT NULL COMMENT 'discount percentage',
  `discount_amnt` decimal(10,2) NOT NULL DEFAULT '0.00',
  `vat_amnt_per` decimal(10,2) NOT NULL DEFAULT '0.00',
  `vat_amnt` decimal(10,2) NOT NULL DEFAULT '0.00',
  `status` int DEFAULT NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- --------------------------------------------------------

--
-- Table structure for table `product_return`
--

CREATE TABLE IF NOT EXISTS `product_return` (
  `return_id` varchar(30) CHARACTER SET latin1 COLLATE latin1_swedish_ci NOT NULL,
  `product_id` varchar(20) CHARACTER SET latin1 COLLATE latin1_swedish_ci NOT NULL,
  `invoice_id` varchar(20) CHARACTER SET latin1 COLLATE latin1_swedish_ci NOT NULL,
  `purchase_id` varchar(30) CHARACTER SET latin1 COLLATE latin1_swedish_ci DEFAULT NULL,
  `date_purchase` varchar(20) CHARACTER SET latin1 COLLATE latin1_swedish_ci NOT NULL,
  `date_return` varchar(30) CHARACTER SET latin1 COLLATE latin1_swedish_ci NOT NULL,
  `byy_qty` float NOT NULL,
  `ret_qty` float NOT NULL,
  `customer_id` varchar(20) CHARACTER SET latin1 COLLATE latin1_swedish_ci NOT NULL,
  `supplier_id` varchar(30) CHARACTER SET latin1 COLLATE latin1_swedish_ci NOT NULL,
  `product_rate` decimal(10,2) NOT NULL DEFAULT '0.00',
  `deduction` float NOT NULL,
  `total_deduct` decimal(10,2) NOT NULL DEFAULT '0.00',
  `total_tax` decimal(10,2) NOT NULL DEFAULT '0.00',
  `total_ret_amount` decimal(10,2) NOT NULL DEFAULT '0.00',
  `net_total_amount` decimal(10,2) NOT NULL DEFAULT '0.00',
  `reason` text CHARACTER SET latin1 COLLATE latin1_swedish_ci NOT NULL,
  `usablity` int NOT NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- --------------------------------------------------------

--
-- Table structure for table `product_service`
--

CREATE TABLE IF NOT EXISTS `product_service` (
  `service_id` int NOT NULL,
  `service_name` varchar(250) NOT NULL,
  `description` text NOT NULL,
  `charge` decimal(10,2) NOT NULL DEFAULT '0.00',
  `service_vat` float DEFAULT NULL,
  `is_fixed` int NOT NULL DEFAULT '0',
  `is_dynamic` int NOT NULL DEFAULT '0',
  `tax0` text,
  `tax1` text
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- --------------------------------------------------------

--
-- Table structure for table `quotation`
--

CREATE TABLE IF NOT EXISTS `quotation` (
  `id` int NOT NULL,
  `quotation_id` varchar(30) NOT NULL,
  `quot_no` varchar(50) NOT NULL,
  `quot_description` text NOT NULL,
  `customer_id` varchar(30) NOT NULL,
  `quotdate` date NOT NULL,
  `expire_date` date DEFAULT NULL,
  `item_total_amount` decimal(12,2) NOT NULL,
  `item_total_dicount` decimal(10,2) NOT NULL DEFAULT '0.00',
  `item_total_vat` double(10,2) NOT NULL DEFAULT '0.00',
  `item_total_tax` decimal(10,2) DEFAULT '0.00',
  `service_total_amount` decimal(10,2) NOT NULL DEFAULT '0.00',
  `service_total_discount` decimal(10,2) NOT NULL DEFAULT '0.00',
  `service_total_vat` double(10,2) NOT NULL DEFAULT '0.00',
  `service_total_tax` decimal(10,2) DEFAULT '0.00',
  `quot_dis_item` decimal(10,2) NOT NULL DEFAULT '0.00',
  `quot_dis_service` decimal(10,2) NOT NULL DEFAULT '0.00',
  `is_fixed` int NOT NULL DEFAULT '0',
  `is_dynamic` int NOT NULL DEFAULT '0',
  `create_by` varchar(30) NOT NULL,
  `create_date` date NOT NULL,
  `update_by` varchar(30) DEFAULT NULL,
  `update_date` date DEFAULT NULL,
  `status` tinyint NOT NULL,
  `cust_show` tinyint DEFAULT NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- --------------------------------------------------------

--
-- Table structure for table `quotation_service_used`
--

CREATE TABLE IF NOT EXISTS `quotation_service_used` (
  `id` int NOT NULL,
  `quot_id` varchar(20) NOT NULL,
  `service_id` int NOT NULL,
  `qty` decimal(10,2) NOT NULL DEFAULT '0.00',
  `charge` decimal(10,2) NOT NULL DEFAULT '0.00',
  `discount` decimal(10,2) NOT NULL DEFAULT '0.00',
  `discount_amount` decimal(10,2) NOT NULL DEFAULT '0.00',
  `vat_per` decimal(10,2) DEFAULT '0.00',
  `vat_amnt` decimal(10,2) DEFAULT '0.00',
  `tax` decimal(10,2) DEFAULT '0.00',
  `total` decimal(10,2) NOT NULL DEFAULT '0.00'
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- --------------------------------------------------------

--
-- Table structure for table `quotation_taxinfo`
--

CREATE TABLE IF NOT EXISTS `quotation_taxinfo` (
  `id` int NOT NULL,
  `date` date NOT NULL,
  `customer_id` int NOT NULL,
  `relation_id` varchar(30) CHARACTER SET utf8mb4 COLLATE utf8mb4_general_ci NOT NULL,
  `tax0` text CHARACTER SET utf8mb4 COLLATE utf8mb4_general_ci,
  `tax1` text CHARACTER SET utf8mb4 COLLATE utf8mb4_general_ci
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- --------------------------------------------------------

--
-- Table structure for table `quot_products_used`
--

CREATE TABLE IF NOT EXISTS `quot_products_used` (
  `id` int NOT NULL,
  `quot_id` varchar(100) NOT NULL,
  `product_id` varchar(100) NOT NULL,
  `serial_no` varchar(30) DEFAULT NULL,
  `batch_id` varchar(10) DEFAULT NULL,
  `description` varchar(250) DEFAULT NULL,
  `used_qty` decimal(10,2) DEFAULT NULL,
  `rate` decimal(10,2) DEFAULT NULL,
  `supplier_rate` float DEFAULT NULL,
  `total_price` decimal(12,2) DEFAULT NULL,
  `discount` decimal(10,2) DEFAULT NULL,
  `discount_per` varchar(15) DEFAULT NULL,
  `vat_amnt` decimal(10,2) DEFAULT NULL,
  `vat_per` decimal(10,2) DEFAULT NULL,
  `tax` decimal(10,2) DEFAULT NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- --------------------------------------------------------

--
-- Table structure for table `role_permission`
--

CREATE TABLE IF NOT EXISTS `role_permission` (
  `id` int NOT NULL,
  `fk_module_id` int NOT NULL,
  `role_id` int NOT NULL,
  `create` tinyint(1) DEFAULT NULL,
  `read` tinyint(1) DEFAULT NULL,
  `update` tinyint(1) DEFAULT NULL,
  `delete` tinyint(1) DEFAULT NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- --------------------------------------------------------

--
-- Table structure for table `salary_setup_header`
--

CREATE TABLE IF NOT EXISTS `salary_setup_header` (
  `s_s_h_id` int UNSIGNED NOT NULL,
  `employee_id` varchar(30) CHARACTER SET latin1 COLLATE latin1_swedish_ci NOT NULL,
  `salary_payable` varchar(30) CHARACTER SET latin1 COLLATE latin1_swedish_ci DEFAULT NULL,
  `absent_deduct` varchar(30) CHARACTER SET latin1 COLLATE latin1_swedish_ci DEFAULT NULL,
  `tax_manager` varchar(30) CHARACTER SET latin1 COLLATE latin1_swedish_ci NOT NULL,
  `status` varchar(30) CHARACTER SET latin1 COLLATE latin1_swedish_ci NOT NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- --------------------------------------------------------

--
-- Table structure for table `salary_sheet_generate`
--

CREATE TABLE IF NOT EXISTS `salary_sheet_generate` (
  `ssg_id` int UNSIGNED NOT NULL,
  `name` varchar(30) CHARACTER SET latin1 COLLATE latin1_swedish_ci DEFAULT NULL,
  `gdate` varchar(30) DEFAULT NULL,
  `start_date` varchar(30) CHARACTER SET latin1 COLLATE latin1_swedish_ci NOT NULL,
  `end_date` varchar(30) CHARACTER SET latin1 COLLATE latin1_swedish_ci NOT NULL,
  `generate_by` varchar(30) CHARACTER SET latin1 COLLATE latin1_swedish_ci NOT NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- --------------------------------------------------------

--
-- Table structure for table `salary_type`
--

CREATE TABLE IF NOT EXISTS `salary_type` (
  `salary_type_id` int NOT NULL,
  `sal_name` varchar(100) NOT NULL,
  `salary_type` varchar(50) NOT NULL,
  `status` varchar(30) NOT NULL,
  `emp_sal_type` varchar(50) DEFAULT NULL,
  `default_amount` varchar(50) DEFAULT NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- --------------------------------------------------------

--
-- Table structure for table `sec_role`
--

CREATE TABLE IF NOT EXISTS `sec_role` (
  `id` int NOT NULL AUTO_INCREMENT,
  `type` varchar(100) CHARACTER SET utf8mb3 COLLATE utf8mb3_general_ci NOT NULL,
  `level` enum('master','admin','manager','employee') COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT 'employee',
  `description` varchar(255) COLLATE utf8mb4_unicode_ci DEFAULT '',
  `is_system` tinyint(1) DEFAULT '0',
  `created_at` datetime DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`)
) ENGINE=InnoDB AUTO_INCREMENT=11 DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

--

--
-- Table structure for table `sec_userrole`
--

CREATE TABLE IF NOT EXISTS `sec_userrole` (
  `id` int NOT NULL AUTO_INCREMENT,
  `user_id` varchar(50) CHARACTER SET utf8mb3 COLLATE utf8mb3_unicode_ci NOT NULL,
  `roleid` int NOT NULL,
  `createby` varchar(50) CHARACTER SET utf8mb3 COLLATE utf8mb3_unicode_ci NOT NULL,
  `createdate` datetime NOT NULL,
  PRIMARY KEY (`id`),
  KEY `idx_sec_userrole_user_id` (`user_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- --------------------------------------------------------

--
-- Table structure for table `seles_termscondi`
--

CREATE TABLE IF NOT EXISTS `seles_termscondi` (
  `id` int NOT NULL,
  `description` text CHARACTER SET utf8mb4 COLLATE utf8mb4_general_ci NOT NULL,
  `status` int NOT NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- --------------------------------------------------------

--
-- Table structure for table `service_invoice`
--

CREATE TABLE IF NOT EXISTS `service_invoice` (
  `id` int NOT NULL,
  `voucher_no` varchar(30) NOT NULL,
  `date` date NOT NULL,
  `employee_id` varchar(100) DEFAULT NULL,
  `customer_id` varchar(30) NOT NULL,
  `total_amount` decimal(20,2) NOT NULL DEFAULT '0.00',
  `total_discount` decimal(20,2) NOT NULL DEFAULT '0.00',
  `invoice_discount` decimal(10,2) NOT NULL DEFAULT '0.00',
  `total_vat_amnt` decimal(10,2) NOT NULL DEFAULT '0.00' COMMENT 'total invoice vat',
  `total_tax` decimal(10,2) DEFAULT '0.00',
  `paid_amount` decimal(10,2) NOT NULL DEFAULT '0.00',
  `due_amount` decimal(10,2) NOT NULL DEFAULT '0.00',
  `shipping_cost` decimal(10,2) NOT NULL DEFAULT '0.00',
  `previous` decimal(10,2) NOT NULL DEFAULT '0.00',
  `details` varchar(250) NOT NULL,
  `sales_by` varchar(20) DEFAULT NULL,
  `is_fixed` int NOT NULL DEFAULT '0',
  `is_dynamic` int NOT NULL DEFAULT '0',
  `is_credit` varchar(10) NOT NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- --------------------------------------------------------

--
-- Table structure for table `service_invoice_details`
--

CREATE TABLE IF NOT EXISTS `service_invoice_details` (
  `id` int NOT NULL,
  `service_id` int NOT NULL,
  `service_inv_id` varchar(30) NOT NULL,
  `qty` decimal(10,2) NOT NULL DEFAULT '0.00',
  `charge` decimal(10,2) NOT NULL DEFAULT '0.00',
  `discount` decimal(10,2) DEFAULT '0.00',
  `discount_amount` decimal(10,2) DEFAULT '0.00',
  `vat` decimal(10,2) DEFAULT '0.00',
  `vat_amnt` decimal(10,2) DEFAULT '0.00',
  `total` decimal(10,2) NOT NULL DEFAULT '0.00'
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- --------------------------------------------------------

--
-- Table structure for table `setting`
--

CREATE TABLE IF NOT EXISTS `setting` (
  `id` int NOT NULL,
  `title` varchar(255) DEFAULT NULL,
  `storename` varchar(100) DEFAULT NULL,
  `address` text,
  `email` varchar(50) DEFAULT NULL,
  `phone` varchar(20) DEFAULT NULL,
  `logo` varchar(50) DEFAULT NULL,
  `logoweb` varchar(255) DEFAULT NULL,
  `favicon` varchar(100) DEFAULT NULL,
  `opentime` varchar(255) DEFAULT NULL,
  `closetime` varchar(255) DEFAULT NULL,
  `vat` int NOT NULL DEFAULT '0',
  `discount_type` int NOT NULL DEFAULT '0' COMMENT '0=amount,1=percent',
  `service_chargeType` int NOT NULL DEFAULT '0' COMMENT '0=amount,1=percent',
  `currency` int DEFAULT '0',
  `min_prepare_time` varchar(50) DEFAULT NULL,
  `language` varchar(100) DEFAULT NULL,
  `timezone` varchar(150) NOT NULL,
  `dateformat` text NOT NULL,
  `site_align` varchar(50) DEFAULT NULL,
  `powerbytxt` text,
  `footer_text` varchar(255) DEFAULT NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- --------------------------------------------------------

--
-- Table structure for table `sms_settings`
--

CREATE TABLE IF NOT EXISTS `sms_settings` (
  `id` int NOT NULL,
  `api_key` varchar(100) DEFAULT NULL,
  `api_secret` varchar(100) DEFAULT NULL,
  `from` varchar(100) DEFAULT NULL,
  `isinvoice` int NOT NULL DEFAULT '0',
  `isservice` int NOT NULL DEFAULT '0',
  `isreceive` int NOT NULL DEFAULT '0'
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- --------------------------------------------------------

--
-- Table structure for table `sub_module`
--

CREATE TABLE IF NOT EXISTS `sub_module` (
  `id` int NOT NULL,
  `mid` int NOT NULL,
  `name` varchar(100) CHARACTER SET utf8mb3 COLLATE utf8mb3_general_ci NOT NULL,
  `description` text CHARACTER SET utf8mb3 COLLATE utf8mb3_general_ci,
  `image` varchar(100) CHARACTER SET utf8mb3 COLLATE utf8mb3_general_ci DEFAULT NULL,
  `directory` varchar(50) CHARACTER SET utf8mb3 COLLATE utf8mb3_general_ci DEFAULT NULL,
  `status` tinyint(1) NOT NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- --------------------------------------------------------

--
-- Table structure for table `supplier_information`
--

CREATE TABLE IF NOT EXISTS `supplier_information` (
  `supplier_id` bigint NOT NULL,
  `supplier_name` varchar(255) NOT NULL,
  `address` varchar(255) DEFAULT NULL,
  `address2` text NOT NULL,
  `mobile` varchar(100) DEFAULT NULL,
  `emailnumber` varchar(200) DEFAULT NULL,
  `email_address` varchar(200) DEFAULT NULL,
  `contact` varchar(100) DEFAULT NULL,
  `phone` varchar(50) DEFAULT NULL,
  `fax` varchar(100) DEFAULT NULL,
  `city` text,
  `state` text,
  `zip` varchar(50) DEFAULT NULL,
  `country` varchar(250) DEFAULT NULL,
  `details` varchar(255) DEFAULT NULL,
  `status` int NOT NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- --------------------------------------------------------

--
-- Table structure for table `supplier_product`
--

CREATE TABLE IF NOT EXISTS `supplier_product` (
  `supplier_pr_id` int NOT NULL,
  `product_id` varchar(30) CHARACTER SET utf8mb3 COLLATE utf8mb3_general_ci NOT NULL,
  `products_model` varchar(30) CHARACTER SET latin1 COLLATE latin1_swedish_ci DEFAULT NULL,
  `supplier_id` bigint NOT NULL,
  `supplier_price` varchar(100) CHARACTER SET utf8mb3 COLLATE utf8mb3_unicode_ci DEFAULT NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- --------------------------------------------------------

--
-- Table structure for table `synchronizer_setting`
--

CREATE TABLE IF NOT EXISTS `synchronizer_setting` (
  `id` int NOT NULL,
  `hostname` varchar(100) NOT NULL,
  `username` varchar(100) NOT NULL,
  `password` varchar(100) NOT NULL,
  `port` varchar(10) NOT NULL,
  `debug` varchar(10) NOT NULL,
  `project_root` varchar(100) NOT NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- --------------------------------------------------------

--
-- Table structure for table `tax_collection`
--

CREATE TABLE IF NOT EXISTS `tax_collection` (
  `id` int NOT NULL,
  `date` date NOT NULL,
  `customer_id` varchar(30) NOT NULL,
  `relation_id` varchar(30) NOT NULL,
  `tax0` text,
  `tax1` text
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- --------------------------------------------------------

--
-- Table structure for table `tax_settings`
--

CREATE TABLE IF NOT EXISTS `tax_settings` (
  `id` int NOT NULL,
  `default_value` float NOT NULL,
  `tax_name` varchar(250) NOT NULL,
  `nt` int NOT NULL,
  `reg_no` varchar(100) NOT NULL,
  `is_show` tinyint NOT NULL DEFAULT '1'
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- --------------------------------------------------------

--
-- Table structure for table `units`
--

CREATE TABLE IF NOT EXISTS `units` (
  `unit_id` int NOT NULL,
  `unit_name` varchar(255) CHARACTER SET latin1 COLLATE latin1_swedish_ci NOT NULL,
  `status` int NOT NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- --------------------------------------------------------

--
-- Table structure for table `users`
--

CREATE TABLE IF NOT EXISTS `users` (
  `id` int NOT NULL,
  `user_id` varchar(15) NOT NULL,
  `last_name` varchar(255) NOT NULL,
  `first_name` varchar(255) NOT NULL,
  `company_name` varchar(250) DEFAULT NULL,
  `address` text,
  `phone` varchar(20) DEFAULT NULL,
  `gender` int DEFAULT NULL,
  `date_of_birth` varchar(255) DEFAULT NULL,
  `logo` varchar(250) DEFAULT NULL,
  `status` int NOT NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- --------------------------------------------------------

--
-- Table structure for table `user_login`
--

CREATE TABLE IF NOT EXISTS `user_login` (
  `id` int NOT NULL,
  `user_id` varchar(15) NOT NULL,
  `username` varchar(255) NOT NULL,
  `password` varchar(255) DEFAULT NULL,
  `user_type` int DEFAULT NULL,
  `security_code` varchar(255) DEFAULT NULL,
  `status` int NOT NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- --------------------------------------------------------

--
-- Table structure for table `vat_tax_setting`
--

CREATE TABLE IF NOT EXISTS `vat_tax_setting` (
  `id` int NOT NULL,
  `dynamic_tax` int NOT NULL DEFAULT '0',
  `fixed_tax` int NOT NULL DEFAULT '0'
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- --------------------------------------------------------

--
-- Table structure for table `web_setting`
--

CREATE TABLE IF NOT EXISTS `web_setting` (
  `setting_id` int NOT NULL,
  `logo` varchar(255) DEFAULT NULL,
  `invoice_logo` varchar(255) DEFAULT NULL,
  `favicon` varchar(255) DEFAULT NULL,
  `currency` varchar(10) DEFAULT NULL,
  `timezone` varchar(150) NOT NULL,
  `currency_position` varchar(10) DEFAULT NULL,
  `footer_text` varchar(255) DEFAULT NULL,
  `language` varchar(255) DEFAULT NULL,
  `rtr` varchar(255) DEFAULT NULL,
  `captcha` int DEFAULT '1' COMMENT '0=active,1=inactive',
  `is_qr` int NOT NULL,
  `is_autoapprove_v` int NOT NULL DEFAULT '0',
  `site_key` varchar(250) DEFAULT NULL,
  `secret_key` varchar(250) DEFAULT NULL,
  `discount_type` int DEFAULT '1'
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;



-- --------------------------------------------------------
--
-- Indexes
--

ALTER TABLE `acc_coa`
  ADD PRIMARY KEY (`id`),
  ADD UNIQUE KEY `HeadCode` (`HeadCode`),
  ADD KEY `HeadName` (`HeadName`),
  ADD KEY `customer_id` (`customer_id`),
  ADD KEY `supplier_id` (`supplier_id`),
  ADD KEY `service_id` (`service_id`),
  ADD KEY `PHeadName` (`PHeadName`),
  ADD KEY `HeadLevel` (`HeadLevel`),
  ADD KEY `IsTransaction` (`IsTransaction`),
  ADD KEY `IsGL` (`IsGL`),
  ADD KEY `IsBudget` (`IsBudget`),
  ADD KEY `IsDepreciation` (`IsDepreciation`),
  ADD KEY `isCashNature` (`isCashNature`),
  ADD KEY `isBankNature` (`isBankNature`);
ALTER TABLE `acc_monthly_balance`
  ADD PRIMARY KEY (`id`);
ALTER TABLE `acc_opening_balance`
  ADD PRIMARY KEY (`id`),
  ADD KEY `COAID` (`COAID`),
  ADD KEY `year` (`fyear`);
ALTER TABLE `acc_predefine_account`
  ADD PRIMARY KEY (`id`),
  ADD KEY `COAID` (`cashCode`),
  ADD KEY `cashCode` (`cashCode`);
ALTER TABLE `acc_subcode`
  ADD PRIMARY KEY (`id`),
  ADD KEY `subTypeId` (`subTypeId`);
ALTER TABLE `acc_subtype`
  ADD PRIMARY KEY (`id`);
ALTER TABLE `acc_transaction`
  ADD UNIQUE KEY `ID` (`ID`),
  ADD KEY `COAID` (`COAID`),
  ADD KEY `VNo` (`VNo`),
  ADD KEY `RevCodde` (`RevCodde`),
  ADD KEY `subType` (`subType`),
  ADD KEY `subCode` (`subCode`),
  ADD KEY `referenceNo` (`referenceNo`),
  ADD KEY `vid` (`vid`);
ALTER TABLE `acc_vaucher`
  ADD PRIMARY KEY (`id`),
  ADD KEY `VNo` (`VNo`),
  ADD KEY `fyear` (`fyear`),
  ADD KEY `VDate` (`VDate`),
  ADD KEY `COAID` (`COAID`),
  ADD KEY `RevCodde` (`RevCodde`),
  ADD KEY `subType` (`subType`),
  ADD KEY `subCode` (`subCode`),
  ADD KEY `referenceNo` (`referenceNo`);
ALTER TABLE `ai_report_types`
  ADD PRIMARY KEY (`id`);
ALTER TABLE `ai_setting`
  ADD PRIMARY KEY (`setting_id`);
ALTER TABLE `app_setting`
  ADD PRIMARY KEY (`id`);
ALTER TABLE `attendance`
  ADD PRIMARY KEY (`att_id`),
  ADD KEY `employee_id` (`employee_id`);
ALTER TABLE `bank_add`
  ADD PRIMARY KEY (`id`);
ALTER TABLE `closing_records`
  ADD PRIMARY KEY (`id`);
ALTER TABLE `currency_tbl`
  ADD PRIMARY KEY (`id`);
ALTER TABLE `customer_information`
  ADD PRIMARY KEY (`customer_id`),
  ADD KEY `customer_id` (`customer_id`);
ALTER TABLE `daily_closing`
  ADD PRIMARY KEY (`closing_id`);
ALTER TABLE `designation`
  ADD PRIMARY KEY (`id`);
ALTER TABLE `email_config`
  ADD PRIMARY KEY (`id`);
ALTER TABLE `employee_history`
  ADD PRIMARY KEY (`id`);
ALTER TABLE `employee_salary_payment`
  ADD PRIMARY KEY (`emp_sal_pay_id`),
  ADD KEY `employee_id` (`employee_id`),
  ADD KEY `generate_id` (`generate_id`);
ALTER TABLE `employee_salary_setup`
  ADD PRIMARY KEY (`e_s_s_id`),
  ADD KEY `employee_id` (`employee_id`);
ALTER TABLE `expense`
  ADD PRIMARY KEY (`id`);
ALTER TABLE `expense_item`
  ADD PRIMARY KEY (`id`);
ALTER TABLE `financial_year`
  ADD PRIMARY KEY (`id`);
ALTER TABLE `gmb_salary_advance`
  ADD PRIMARY KEY (`id`);
ALTER TABLE `gmb_salary_generate`
  ADD PRIMARY KEY (`id`);
ALTER TABLE `gmb_salary_sheet_generate`
  ADD PRIMARY KEY (`ssg_id`);
ALTER TABLE `invoice`
  ADD PRIMARY KEY (`id`),
  ADD KEY `customer_id` (`customer_id`),
  ADD KEY `invoice_id` (`invoice_id`);
ALTER TABLE `invoice_details`
  ADD PRIMARY KEY (`id`),
  ADD KEY `invoice_id` (`invoice_id`),
  ADD KEY `product_id` (`product_id`);
ALTER TABLE `language`
  ADD PRIMARY KEY (`id`);
ALTER TABLE `module`
  ADD PRIMARY KEY (`id`);
ALTER TABLE `module_purchase_key`
  ADD PRIMARY KEY (`id`);
ALTER TABLE `payroll_tax_setup`
  ADD PRIMARY KEY (`tax_setup_id`);
ALTER TABLE `personal_loan`
  ADD PRIMARY KEY (`per_loan_id`);
ALTER TABLE `person_information`
  ADD PRIMARY KEY (`id`);
ALTER TABLE `person_ledger`
  ADD PRIMARY KEY (`id`),
  ADD KEY `person_id` (`person_id`);
ALTER TABLE `pesonal_loan_information`
  ADD PRIMARY KEY (`id`),
  ADD KEY `person_id` (`person_id`);
ALTER TABLE `print_setting`
  ADD PRIMARY KEY (`id`);
ALTER TABLE `product_category`
  ADD PRIMARY KEY (`category_id`);
ALTER TABLE `product_information`
  ADD PRIMARY KEY (`id`),
  ADD KEY `category_id` (`category_id`),
  ADD KEY `product_id` (`product_id`);
ALTER TABLE `product_purchase`
  ADD PRIMARY KEY (`id`),
  ADD KEY `purchase_id` (`purchase_id`),
  ADD KEY `supplier_id` (`supplier_id`);
ALTER TABLE `product_purchase_details`
  ADD PRIMARY KEY (`id`),
  ADD KEY `purchase_id` (`purchase_id`),
  ADD KEY `product_id` (`product_id`);
ALTER TABLE `product_return`
  ADD KEY `product_id` (`product_id`),
  ADD KEY `invoice_id` (`invoice_id`),
  ADD KEY `purchase_id` (`purchase_id`),
  ADD KEY `customer_id` (`customer_id`),
  ADD KEY `supplier_id` (`supplier_id`);
ALTER TABLE `product_service`
  ADD PRIMARY KEY (`service_id`);
ALTER TABLE `quotation`
  ADD PRIMARY KEY (`id`),
  ADD UNIQUE KEY `quot_no` (`quot_no`),
  ADD KEY `quotation_id` (`quotation_id`),
  ADD KEY `customer_id` (`customer_id`);
ALTER TABLE `quotation_service_used`
  ADD PRIMARY KEY (`id`),
  ADD KEY `quot_id` (`quot_id`),
  ADD KEY `service_id` (`service_id`);
ALTER TABLE `quotation_taxinfo`
  ADD PRIMARY KEY (`id`),
  ADD KEY `customer_id` (`customer_id`);
ALTER TABLE `quot_products_used`
  ADD PRIMARY KEY (`id`),
  ADD KEY `product_id` (`product_id`),
  ADD KEY `quot_id` (`quot_id`);
ALTER TABLE `role_permission`
  ADD PRIMARY KEY (`id`),
  ADD KEY `fk_module_id` (`fk_module_id`),
  ADD KEY `fk_user_id` (`role_id`);
ALTER TABLE `salary_sheet_generate`
  ADD PRIMARY KEY (`ssg_id`);
ALTER TABLE `salary_type`
  ADD PRIMARY KEY (`salary_type_id`);
ALTER TABLE `sec_role`
  ADD PRIMARY KEY (`id`);
ALTER TABLE `sec_userrole`
  ADD UNIQUE KEY `ID` (`id`);
ALTER TABLE `seles_termscondi`
  ADD PRIMARY KEY (`id`);
ALTER TABLE `service_invoice`
  ADD PRIMARY KEY (`id`),
  ADD KEY `customer_id` (`customer_id`);
ALTER TABLE `service_invoice_details`
  ADD PRIMARY KEY (`id`),
  ADD KEY `service_id` (`service_id`);
ALTER TABLE `setting`
  ADD PRIMARY KEY (`id`);
ALTER TABLE `sms_settings`
  ADD PRIMARY KEY (`id`);
ALTER TABLE `sub_module`
  ADD PRIMARY KEY (`id`);
ALTER TABLE `supplier_information`
  ADD PRIMARY KEY (`supplier_id`),
  ADD KEY `supplier_id` (`supplier_id`);
ALTER TABLE `supplier_product`
  ADD PRIMARY KEY (`supplier_pr_id`),
  ADD KEY `product_id` (`product_id`),
  ADD KEY `supplier_id` (`supplier_id`);
ALTER TABLE `tax_collection`
  ADD PRIMARY KEY (`id`),
  ADD KEY `customer_id` (`customer_id`);
ALTER TABLE `tax_settings`
  ADD PRIMARY KEY (`id`);
ALTER TABLE `units`
  ADD PRIMARY KEY (`unit_id`);
ALTER TABLE `users`
  ADD PRIMARY KEY (`id`);
ALTER TABLE `user_login`
  ADD PRIMARY KEY (`id`);
ALTER TABLE `vat_tax_setting`
  ADD PRIMARY KEY (`id`);
ALTER TABLE `web_setting`
  ADD PRIMARY KEY (`setting_id`);

-- --------------------------------------------------------
--
-- Seed data: accounting backbone
--

INSERT INTO `acc_coa` (`id`, `HeadCode`, `HeadName`, `PHeadName`, `PHeadCode`, `HeadLevel`, `IsActive`, `IsTransaction`, `IsGL`, `isCashNature`, `isBankNature`, `HeadType`, `IsBudget`, `IsDepreciation`, `customer_id`, `supplier_id`, `bank_id`, `service_id`, `DepreciationRate`, `CreateBy`, `CreateDate`, `UpdateBy`, `UpdateDate`, `isSubType`, `subType`, `isStock`, `isFixedAssetSch`, `noteNo`, `assetCode`, `depCode`) VALUES
(8, 50202, 'Accounts Payable', 'Current Liabilities', '502', 3, 1, 0, 1, 0, 0, 'L', 0, 0, NULL, NULL, NULL, NULL, 0.00, '1', '2022-05-17 12:52:17', '0', '1970-01-01 00:00:00', 0, 1, 0, 0, NULL, NULL, NULL),
(10, 1, 'Assets', 'COA', '0', 1, 1, 0, 0, 0, 0, 'A', 0, 0, NULL, NULL, NULL, NULL, 0.00, '0', '1970-01-01 00:00:00', '0', '1970-01-01 00:00:00', 0, 1, 0, 0, '', '', ''),
(13, 10201, 'Cash', 'Current Asset', '102', 3, 1, 0, 0, 0, 0, 'A', 0, 0, NULL, NULL, NULL, NULL, 0.00, '1', '2022-04-10 12:07:11', '0', '2015-10-15 15:57:55', 0, 1, 0, 0, NULL, NULL, NULL),
(15, 1020101, 'Cash In Hand', 'Cash', '10201', 4, 1, 1, 1, 1, 0, 'A', 0, 0, NULL, NULL, NULL, NULL, 0.00, '1', '2022-04-12 08:16:03', '0', '2016-05-23 12:05:43', 0, 1, 0, 0, NULL, NULL, NULL),
(16, 102, 'Current Asset', 'Assets', '1', 2, 1, 0, 0, 0, 0, 'A', 0, 0, NULL, NULL, NULL, NULL, 0.00, '0', '1970-01-01 00:00:00', '0', '2018-07-07 11:23:00', 0, 1, 0, 0, '', '', ''),
(17, 502, 'Current Liabilities', 'Liabilities', '5', 2, 1, 0, 0, 0, 0, 'L', 0, 0, NULL, NULL, NULL, NULL, 0.00, '0', '2014-08-30 13:18:20', '0', '2015-10-15 19:49:21', 0, 1, 0, 0, '', '', ''),
(23, 401, 'Cost of Goods Solds', 'Expenses', '4', 2, 1, 1, 1, 0, 0, 'E', 0, 0, NULL, NULL, NULL, NULL, 0.00, '1', '2022-04-02 10:28:34', '0', '1970-01-01 00:00:00', 0, 1, 0, 0, '', '', ''),
(24, 2, 'Shareholder\'s Equity', 'COA', '0', 1, 1, 0, 0, 0, 0, 'L', 0, 0, NULL, NULL, NULL, NULL, 0.00, '0', '1970-01-01 00:00:00', '0', '1970-01-01 00:00:00', 0, 1, 0, 0, '', '', ''),
(25, 4, 'Expenses', 'COA', '0', 1, 1, 0, 0, 0, 0, 'E', 0, 0, NULL, NULL, NULL, NULL, 0.00, '2', '2019-11-24 05:45:24', '0', '1970-01-01 00:00:00', 0, 1, 0, 0, '', '', ''),
(26, 101, 'Fixed Assets', 'Assets', '1', 2, 1, 0, 0, 0, 0, 'A', 0, 0, NULL, NULL, NULL, NULL, 0.00, '0', '1970-01-01 00:00:00', '0', '2015-10-15 15:29:11', 0, 1, 0, 0, '', '', ''),
(27, 402, 'Over Head Cost', 'Expenses', '4', 2, 1, 1, 1, 0, 0, 'E', 0, 0, NULL, NULL, NULL, NULL, 0.00, '1', '2022-04-04 10:01:58', '0', '1970-01-01 00:00:00', 0, 1, 0, 0, '', '', ''),
(29, 3, 'Income', 'COA', '0', 1, 1, 0, 0, 0, 0, 'I', 0, 0, NULL, NULL, NULL, NULL, 0.00, '0', '1970-01-01 00:00:00', '0', '1970-01-01 00:00:00', 0, 1, 0, 0, '', '', ''),
(30, 5, 'Liabilities', 'COA', '0', 1, 1, 0, 0, 0, 0, 'L', 0, 0, NULL, NULL, NULL, NULL, 0.00, '0', '2013-07-04 12:32:07', '0', '2015-10-15 19:46:54', 0, 1, 0, 0, '', '', ''),
(31, 501, 'Long Term Liabilities', 'Liabilities', '5', 2, 1, 0, 0, 0, 0, 'L', 0, 0, NULL, NULL, NULL, NULL, 0.00, '0', '2014-08-30 13:18:20', '0', '2015-10-15 19:49:21', 0, 1, 0, 0, '', '', ''),
(33, 301, 'Direct Income', 'Income', '3', 2, 1, 1, 1, 0, 0, 'I', 0, 0, NULL, NULL, NULL, NULL, 0.00, '1', '2022-04-02 10:31:45', '0', '1970-01-01 00:00:00', 0, 1, 0, 0, '', '', ''),
(35, 302, 'Indirect Income', 'Income', '3', 2, 1, 1, 1, 0, 0, 'I', 0, 0, NULL, NULL, NULL, NULL, 0.00, '1', '2022-04-02 10:55:45', '0', '1970-01-01 00:00:00', 0, 1, 0, 0, '', '', ''),
(36, 5020201, 'Supplier Payable', 'Accounts Payable', '50202', 4, 1, 0, 1, 0, 0, 'L', 0, 0, NULL, NULL, NULL, NULL, 0.00, '1', '2022-05-17 12:52:44', '0', '1970-01-01 00:00:00', 1, 4, 0, 0, NULL, NULL, NULL),
(41, 10203, 'Prepaid Expenses', 'Current Asset', '102', 3, 1, 0, 0, 0, 0, 'A', 0, 0, NULL, NULL, NULL, NULL, 0.00, '1', '2022-04-10 06:45:19', '0', '1970-01-01 00:00:00', 0, 1, 0, 0, NULL, NULL, NULL),
(43, 10204, 'Inventory', 'Current Asset', '102', 3, 1, 0, 0, 0, 0, 'A', 0, 0, NULL, NULL, NULL, NULL, 0.00, '1', '2022-04-10 06:48:32', '0', '1970-01-01 00:00:00', 0, 1, 0, 0, NULL, NULL, NULL),
(44, 50203, 'Accrued Expenses', 'Current Liabilities', '502', 3, 1, 0, 0, 0, 0, 'L', 0, 0, NULL, NULL, NULL, NULL, 0.00, '1', '2022-04-10 06:50:20', '0', '1970-01-01 00:00:00', 0, 1, 0, 0, NULL, NULL, NULL),
(46, 50101, 'Long-Term Debt', 'Long Term Liabilities', '501', 3, 1, 0, 0, 0, 0, 'L', 0, 0, NULL, NULL, NULL, NULL, 0.00, '1', '2022-04-10 06:51:45', '0', '1970-01-01 00:00:00', 0, 1, 0, 0, NULL, NULL, NULL),
(47, 50102, 'Other Long-Term  Liabilities', 'Long Term Liabilities', '501', 3, 1, 0, 0, 0, 0, 'L', 0, 0, NULL, NULL, NULL, NULL, 0.00, '1', '2022-04-10 06:53:04', '0', '1970-01-01 00:00:00', 0, 1, 0, 0, NULL, NULL, NULL),
(48, 201, 'Equity', 'Shareholder\'s Equity', '2', 2, 1, 0, 0, 0, 0, 'L', 0, 0, NULL, NULL, NULL, NULL, 0.00, '0', '2022-04-10 06:56:38', '0', '2022-04-10 06:56:38', 0, 1, 0, 0, NULL, NULL, NULL),
(49, 20101, 'Equity Capital', 'Equity', '201', 3, 1, 0, 0, 0, 0, 'L', 0, 0, NULL, NULL, NULL, NULL, 0.00, '1', '2022-05-17 12:31:33', '0', '1970-01-01 00:00:00', 0, 1, 0, 0, NULL, NULL, NULL),
(50, 20102, 'Retained Earnings', 'Equity', '201', 3, 1, 0, 0, 0, 0, 'L', 0, 0, NULL, NULL, NULL, NULL, 0.00, '1', '2022-04-10 07:01:45', '0', '1970-01-01 00:00:00', 0, 1, 0, 0, NULL, NULL, NULL),
(51, 10101, 'Property & Equipment', 'Fixed Assets', '101', 3, 1, 0, 0, 0, 0, 'A', 0, 0, NULL, NULL, NULL, NULL, 0.00, '1', '2022-04-10 11:35:53', '0', '1970-01-01 00:00:00', 0, 1, 0, 0, NULL, NULL, NULL),
(52, 10102, 'Goodwills', 'Fixed Assets', '101', 3, 1, 0, 0, 0, 0, 'A', 0, 0, NULL, NULL, NULL, NULL, 0.00, '1', '2022-04-23 06:47:21', '0', '1970-01-01 00:00:00', 0, 1, 0, 0, NULL, NULL, NULL),
(53, 30101, 'Construction Income', 'Direct Income', '301', 3, 1, 0, 0, 0, 0, 'I', 0, 0, NULL, NULL, NULL, NULL, 0.00, '1', '2022-04-10 08:08:04', '0', '1970-01-01 00:00:00', 0, 1, 0, 0, NULL, NULL, NULL),
(54, 30102, 'Reimbursement Income', 'Direct Income', '301', 3, 1, 0, 0, 0, 0, 'I', 0, 0, NULL, NULL, NULL, NULL, 0.00, '1', '2022-04-10 08:09:02', '0', '1970-01-01 00:00:00', 0, 1, 0, 0, NULL, NULL, NULL),
(55, 40101, 'Cost of Goods Sold', 'Cost of Goods Solds', '401', 3, 1, 0, 0, 0, 0, 'E', 0, 0, NULL, NULL, NULL, NULL, 0.00, '1', '2022-04-10 08:13:52', '0', '1970-01-01 00:00:00', 0, 1, 0, 0, NULL, NULL, NULL),
(56, 40102, 'Job Expenses', 'Cost of Goods Solds', '401', 3, 1, 0, 0, 0, 0, 'E', 0, 0, NULL, NULL, NULL, NULL, 0.00, '1', '2022-04-10 08:14:18', '0', '1970-01-01 00:00:00', 0, 1, 0, 0, NULL, NULL, NULL),
(57, 40201, 'Automobile', 'Over Head Cost', '402', 3, 1, 0, 0, 0, 0, 'E', 0, 0, NULL, NULL, NULL, NULL, 0.00, '1', '2022-04-10 08:14:37', '0', '1970-01-01 00:00:00', 0, 1, 0, 0, NULL, NULL, NULL),
(58, 40202, 'Bank Service Charges', 'Over Head Cost', '402', 3, 1, 0, 0, 0, 0, 'E', 0, 0, NULL, NULL, NULL, NULL, 0.00, '1', '2022-04-10 08:15:32', '0', '1970-01-01 00:00:00', 0, 1, 0, 0, NULL, NULL, NULL),
(59, 40203, 'Insurance', 'Over Head Cost', '402', 3, 1, 0, 0, 0, 0, 'E', 0, 0, NULL, NULL, NULL, NULL, 0.00, '1', '2022-04-10 08:15:58', '0', '1970-01-01 00:00:00', 0, 1, 0, 0, NULL, NULL, NULL),
(60, 40204, 'Interest Expenses', 'Over Head Cost', '402', 3, 1, 0, 0, 0, 0, 'E', 0, 0, NULL, NULL, NULL, NULL, 0.00, '1', '2022-04-10 08:16:36', '0', '1970-01-01 00:00:00', 0, 1, 0, 0, NULL, NULL, NULL),
(61, 40205, 'Payroll Expenses', 'Over Head Cost', '402', 3, 1, 0, 0, 0, 0, 'E', 0, 0, NULL, NULL, NULL, NULL, 0.00, '1', '2022-04-10 08:17:08', '0', '1970-01-01 00:00:00', 0, 1, 0, 0, NULL, NULL, NULL),
(62, 40206, 'Postage', 'Over Head Cost', '402', 3, 1, 0, 0, 0, 0, 'E', 0, 0, NULL, NULL, NULL, NULL, 0.00, '1', '2022-04-10 08:17:26', '0', '1970-01-01 00:00:00', 0, 1, 0, 0, NULL, NULL, NULL),
(63, 40207, 'Professional Fees', 'Over Head Cost', '402', 3, 1, 0, 0, 0, 0, 'E', 0, 0, NULL, NULL, NULL, NULL, 0.00, '1', '2022-04-10 08:17:55', '0', '1970-01-01 00:00:00', 0, 1, 0, 0, NULL, NULL, NULL),
(64, 40208, 'Repairs', 'Over Head Cost', '402', 3, 1, 0, 0, 0, 0, 'E', 0, 0, NULL, NULL, NULL, NULL, 0.00, '1', '2022-04-10 08:18:38', '0', '1970-01-01 00:00:00', 0, 1, 0, 0, NULL, NULL, NULL),
(65, 40209, 'Tools and Macchnery', 'Over Head Cost', '402', 3, 1, 0, 0, 0, 0, 'E', 0, 0, NULL, NULL, NULL, NULL, 0.00, '1', '2022-04-10 08:28:02', '0', '1970-01-01 00:00:00', 0, 1, 0, 0, NULL, NULL, NULL),
(66, 40210, 'Utilities', 'Over Head Cost', '402', 3, 1, 0, 0, 0, 0, 'E', 0, 0, NULL, NULL, NULL, NULL, 0.00, '1', '2022-04-10 08:28:42', '0', '1970-01-01 00:00:00', 0, 1, 0, 0, NULL, NULL, NULL),
(67, 4021001, 'Electic Bill', 'Utilities', '40210', 4, 1, 0, 0, 0, 0, 'E', 0, 0, NULL, NULL, NULL, NULL, 0.00, '1', '2022-04-10 09:05:45', '0', '1970-01-01 00:00:00', 0, 1, 0, 0, NULL, NULL, NULL),
(68, 4021002, 'House Rent', 'Utilities', '40210', 4, 1, 0, 0, 0, 0, 'E', 0, 0, NULL, NULL, NULL, NULL, 0.00, '1', '2022-04-10 09:06:05', '0', '1970-01-01 00:00:00', 0, 1, 0, 0, NULL, NULL, NULL),
(69, 10205, 'Cash at Bank', 'Current Asset', '102', 3, 1, 0, 0, 0, 0, 'A', 0, 0, NULL, NULL, NULL, NULL, 0.00, '1', '2022-05-17 12:44:19', '0', '1970-01-01 00:00:00', 0, 1, 0, 0, NULL, NULL, NULL),
(75, 1010201, 'Goodwill', 'Goodwills', '10102', 4, 1, 0, 0, 0, 0, 'A', 0, 0, NULL, NULL, NULL, NULL, 15.00, '1', '2022-04-23 06:45:48', '0', '1970-01-01 00:00:00', 0, 1, 0, 1, NULL, 'GD001', NULL),
(77, 5020401, 'property sales', 'Unearned Revenue', '50204', 4, 1, 0, 0, 0, 0, 'L', 0, 0, NULL, NULL, NULL, NULL, 0.00, '1', '2022-04-10 11:40:48', '0', '1970-01-01 00:00:00', 0, 1, 0, 0, NULL, NULL, NULL),
(78, 5010101, 'Debts', 'Long-Term Debt', '50101', 4, 1, 0, 0, 0, 0, 'L', 0, 0, NULL, NULL, NULL, NULL, 0.00, '1', '2022-04-10 11:41:49', '0', '1970-01-01 00:00:00', 0, 1, 0, 0, NULL, NULL, NULL),
(79, 5010201, 'Other Long-Term  Liabilities', 'Other Long-Term  Liabilities', '50102', 4, 1, 0, 0, 0, 0, 'L', 0, 0, NULL, NULL, NULL, NULL, 0.00, '1', '2022-04-10 11:42:03', '0', '1970-01-01 00:00:00', 0, 1, 0, 0, NULL, NULL, NULL),
(80, 2010101, 'Capital Fund', 'Equity Capital', '20101', 4, 1, 0, 0, 0, 0, 'L', 0, 0, NULL, NULL, NULL, NULL, 0.00, '1', '2022-04-10 11:42:36', '0', '1970-01-01 00:00:00', 0, 1, 0, 0, NULL, NULL, NULL),
(81, 2010201, 'Current year Profit & Loss', 'Retained Earnings', '20102', 4, 1, 0, 0, 0, 0, 'L', 0, 0, NULL, NULL, NULL, NULL, 0.00, '1', '2022-04-10 11:42:53', '0', '1970-01-01 00:00:00', 0, 1, 0, 0, NULL, NULL, NULL),
(82, 2010202, 'Last year Profit & Loss', 'Retained Earnings', '20102', 4, 1, 0, 0, 0, 0, 'L', 0, 0, NULL, NULL, NULL, NULL, 0.00, '1', '2022-04-10 11:43:03', '0', '1970-01-01 00:00:00', 0, 1, 0, 0, NULL, NULL, NULL),
(84, 50201, 'Provisions', 'Current Liabilities', '502', 3, 1, 0, 0, 0, 0, 'L', 0, 0, NULL, NULL, NULL, NULL, 0.00, '1', '2022-04-10 11:46:00', '0', '1970-01-01 00:00:00', 0, 1, 0, 0, NULL, NULL, NULL),
(85, 5020104, 'Depreciation of Goodwill', 'Depreciations', '50205', 4, 1, 0, 0, 0, 0, 'L', 0, 0, NULL, NULL, NULL, NULL, 0.00, '1', '2022-04-23 06:47:07', '0', '1970-01-01 00:00:00', 0, 1, 0, 1, NULL, NULL, 'GD001'),
(86, 50204, 'Unearned Revenue', 'Current Liabilities', '502', 3, 1, 0, 0, 0, 0, 'L', 0, 0, NULL, NULL, NULL, NULL, 0.00, '1', '2022-05-17 12:53:09', '0', '1970-01-01 00:00:00', 0, 1, 0, 0, NULL, NULL, NULL),
(88, 10206, 'Advance', 'Current Asset', '102', 3, 1, 0, 0, 0, 0, 'A', 0, 0, NULL, NULL, NULL, NULL, 0.00, '1', '2022-04-11 11:56:56', '0', '1970-01-01 00:00:00', 0, 1, 0, 0, NULL, NULL, NULL),
(89, 1020601, 'Advance against Employee', 'Advance', '10206', 4, 1, 0, 0, 0, 0, 'A', 0, 0, NULL, NULL, NULL, NULL, 0.00, '1', '2022-04-11 11:57:18', '0', '1970-01-01 00:00:00', 1, 2, 0, 0, NULL, NULL, NULL),
(90, 1020602, 'Advance Against Customer', 'Advance', '10206', 4, 1, 0, 0, 0, 0, 'A', 0, 0, NULL, NULL, NULL, NULL, 0.00, '1', '2022-04-11 11:57:35', '0', '1970-01-01 00:00:00', 1, 3, 0, 0, NULL, NULL, NULL),
(91, 1020102, 'Petty Cash', 'Cash', '10201', 4, 1, 0, 0, 1, 0, 'A', 0, 0, NULL, NULL, NULL, NULL, 0.00, '1', '2022-04-12 08:16:19', '0', '1970-01-01 00:00:00', 0, 1, 0, 0, NULL, NULL, NULL),
(94, 40301, 'Purchase Account', 'Purchase Accounts', '402', 3, 1, 0, 0, 0, 0, 'E', 0, 0, NULL, NULL, NULL, NULL, 0.00, '0', '2022-04-16 10:31:43', '0', '2022-04-16 10:31:43', 0, 1, 0, 0, NULL, NULL, NULL),
(95, 4030102, 'Purchase', 'Purchase Account', '40201', 4, 1, 0, 0, 0, 0, 'E', 0, 0, NULL, NULL, NULL, NULL, 0.00, '1', '2022-04-16 10:33:59', '0', '1970-01-01 00:00:00', 0, 1, 0, 0, NULL, NULL, NULL),
(96, 30103, 'Sales Accounts', 'Direct Income', '301', 3, 1, 0, 0, 0, 0, 'I', 0, 0, NULL, NULL, NULL, NULL, 0.00, '1', '2022-04-16 10:34:37', '0', '1970-01-01 00:00:00', 0, 1, 0, 0, NULL, NULL, NULL),
(97, 3010301, 'Sales Account', 'Sales Accounts', '30103', 4, 1, 0, 0, 0, 0, 'I', 0, 0, NULL, NULL, NULL, NULL, 0.00, '1', '2022-04-16 10:34:57', '0', '1970-01-01 00:00:00', 0, 1, 0, 0, NULL, NULL, NULL),
(99, 4020501, 'Salary Expense', 'Payroll Expenses', '40205', 4, 1, 0, 0, 0, 0, 'E', 0, 0, NULL, NULL, NULL, NULL, 0.00, '1', '2022-04-20 06:24:08', '0', '1970-01-01 00:00:00', 0, 1, 0, 0, NULL, NULL, NULL),
(104, 5020102, 'Provision for npf contribution', 'Provisions', '50201', 4, 1, 0, 0, 0, 0, 'L', 0, 0, NULL, NULL, NULL, NULL, 0.00, '1', '2022-05-10 06:19:45', '0', '1970-01-01 00:00:00', 0, 1, 0, 0, NULL, NULL, NULL),
(106, 5020101, 'Provision for State Income Tax', 'Provisions', '50201', 4, 1, 0, 0, 0, 0, 'L', 0, 0, NULL, NULL, NULL, NULL, 0.00, '1', '2022-04-26 06:44:29', '0', '1970-01-01 00:00:00', 0, 1, 0, 0, NULL, NULL, NULL),
(107, 40211, 'State Income Tax', 'Over Head Cost', '402', 3, 1, 0, 0, 0, 0, 'E', 0, 0, NULL, NULL, NULL, NULL, 0.00, '1', '2022-04-26 06:44:46', '0', '1970-01-01 00:00:00', 0, 1, 0, 0, NULL, NULL, NULL),
(108, 4021101, 'State Income Tax', 'State Income Tax', '40211', 4, 1, 0, 0, 0, 0, 'E', 0, 0, NULL, NULL, NULL, NULL, 0.00, '1', '2022-04-26 06:45:00', '0', '1970-01-01 00:00:00', 0, 1, 0, 0, NULL, NULL, NULL),
(110, 40212, 'Employeer ICF Expense', 'Over Head Cost', '402', 3, 1, 0, 0, 0, 0, 'E', 0, 0, NULL, NULL, NULL, NULL, 0.00, '1', '2022-05-10 06:35:37', '0', '1970-01-01 00:00:00', 0, 1, 0, 0, NULL, NULL, NULL),
(111, 4021201, 'Employeer 1% ICF Expense', 'Employeer ICF Expense', '40212', 4, 1, 0, 0, 0, 0, 'E', 0, 0, NULL, NULL, NULL, NULL, 0.00, '1', '2022-05-17 11:34:02', '0', '1970-01-01 00:00:00', 0, 1, 0, 0, NULL, NULL, NULL),
(112, 50205, 'Depreciations', 'Current Liabilities', '502', 3, 1, 0, 0, 0, 0, 'L', 0, 0, NULL, NULL, NULL, NULL, 0.00, '0', '2022-05-10 09:35:15', '0', '2022-05-10 09:35:15', 0, 1, 0, 0, NULL, NULL, NULL),
(191, 4020502, 'Employee 5 % NPF Expenses', 'Payroll Expenses', '40205', 4, 1, 0, 0, 0, 0, 'E', 0, 0, NULL, NULL, NULL, NULL, 0.00, '1', '2022-05-17 11:32:14', '0', '1970-01-01 00:00:00', 0, 1, 0, 0, NULL, NULL, NULL),
(192, 4020503, 'Employee 10 % NPF Expenses', 'Payroll Expenses', '40205', 4, 1, 0, 0, 0, 0, 'E', 0, 0, NULL, NULL, NULL, NULL, 0.00, '1', '2022-05-17 11:32:36', '0', '1970-01-01 00:00:00', 0, 1, 0, 0, NULL, NULL, NULL),
(221, 50206, 'Aoounts pay by name supplier', 'Current Liabilities', '502', 3, 1, 0, 0, 0, 0, 'L', 0, 0, NULL, NULL, NULL, NULL, 0.00, '1', '2022-06-13 11:51:32', '0', '1970-01-01 00:00:00', 0, 1, 0, 0, NULL, NULL, NULL),
(229, 10208, 'Accounts Receivable', 'Current Asset', '102', 3, 1, 0, 0, 0, 0, 'A', 0, 0, NULL, NULL, NULL, NULL, 0.00, '1', '2022-06-16 07:05:42', '0', '1970-01-01 00:00:00', 0, 1, 0, 0, NULL, NULL, NULL),
(230, 1020801, 'Customer Receivable', 'Accounts Receivable', '10208', 4, 1, 0, 0, 0, 0, 'A', 0, 0, NULL, NULL, NULL, NULL, 0.00, '1', '2022-06-16 07:07:17', '0', '1970-01-01 00:00:00', 1, 3, 0, 0, NULL, NULL, NULL),
(231, 1020802, 'Employee Receivable', 'Accounts Receivable', '10208', 4, 1, 0, 0, 0, 0, 'A', 0, 0, NULL, NULL, NULL, NULL, 0.00, '1', '2022-06-16 07:06:58', '0', '1970-01-01 00:00:00', 1, 2, 0, 0, NULL, NULL, NULL),
(232, 1020401, 'Inventory account', 'Inventory', '10204', 4, 1, 0, 0, 0, 0, 'A', 0, 0, NULL, NULL, NULL, NULL, 0.00, '1', '2022-06-15 07:53:16', '', '1970-01-01 00:00:00', 0, 1, 0, 0, NULL, NULL, NULL),
(233, 4010101, 'Cost of Goods Sold Account', 'Cost of Goods Sold', '40101', 4, 1, 0, 0, 0, 0, 'E', 0, 0, NULL, NULL, NULL, NULL, 0.00, '1', '2022-06-15 08:58:18', '', '1970-01-01 00:00:00', 0, 1, 0, 0, NULL, NULL, NULL),
(234, 30104, 'Service Accounts', 'Direct Income', '301', 3, 1, 0, 0, 0, 0, 'I', 0, 0, NULL, NULL, NULL, NULL, 0.00, '1', '2022-06-15 16:23:37', '', '1970-01-01 00:00:00', 0, 1, 0, 0, NULL, NULL, NULL),
(235, 3010401, 'Service Account', 'Service Accounts', '30104', 4, 1, 0, 0, 0, 0, 'I', 0, 0, NULL, NULL, NULL, NULL, 0.00, '1', '2022-06-15 16:24:11', '', '1970-01-01 00:00:00', 0, 1, 0, 0, NULL, NULL, NULL),
(241, 1020501, 'fdsfsd', 'Cash at Bank', '10205', 4, 1, 0, 0, 0, 1, 'A', 0, 0, '0', '0', '0', '0', 0.00, '1', '2025-05-12 11:07:36', '0', '1970-01-01 00:00:00', 0, 1, 0, 0, '0', '0', '0'),
(242, 1020502, 'test', 'Cash at Bank', '10205', 4, 1, 0, 0, 0, 1, 'A', 0, 0, '0', '0', '0', '0', 0.00, '1', '2025-05-12 11:34:42', '0', '1970-01-01 00:00:00', 0, 1, 0, 0, '0', '0', '0');

INSERT INTO `acc_predefine_account` (`id`, `cashCode`, `bankCode`, `advance`, `fixedAsset`, `purchaseCode`, `salesCode`, `serviceCode`, `customerCode`, `supplierCode`, `costs_of_good_solds`, `vat`, `tax`, `inventoryCode`, `CPLCode`, `LPLCode`, `salary_code`, `emp_npf_contribution`, `empr_npf_contribution`, `emp_icf_contribution`, `empr_icf_contribution`, `prov_state_tax`, `state_tax`, `prov_npfcode`) VALUES
(2, 10201, 10205, 10206, 101, 1020401, 3010301, 3010401, 1020801, 5020201, 4010101, 0, 4021101, 1020401, 2010201, 2010202, 4020501, 4020502, 4020503, 4021201, 0, 5020101, 4021101, 5020102);

INSERT INTO `acc_subtype` (`id`, `subtypeName`, `staus`, `created_by`, `created_date`) VALUES
(1, 'None', 1, 1, '2022-04-05 10:19:27'),
(2, 'Employee', 1, 1, '2022-04-06 08:14:48'),
(3, 'Customer', 1, 1, '2022-04-10 08:49:22'),
(4, 'Supplier', 1, 1, '2022-04-10 08:49:22'),
(6, 'Agent', 1, 1, '2022-04-10 08:50:12');


-- --------------------------------------------------------
--
-- AUTO_INCREMENT columns
--

ALTER TABLE `acc_coa`
  MODIFY `id` int NOT NULL AUTO_INCREMENT, AUTO_INCREMENT=243;
ALTER TABLE `acc_monthly_balance`
  MODIFY `id` int NOT NULL AUTO_INCREMENT;
ALTER TABLE `acc_opening_balance`
  MODIFY `id` int NOT NULL AUTO_INCREMENT;
ALTER TABLE `acc_predefine_account`
  MODIFY `id` int NOT NULL AUTO_INCREMENT, AUTO_INCREMENT=3;
ALTER TABLE `acc_subcode`
  MODIFY `id` int NOT NULL AUTO_INCREMENT, AUTO_INCREMENT=9;
ALTER TABLE `acc_transaction`
  MODIFY `ID` int NOT NULL AUTO_INCREMENT;
ALTER TABLE `acc_vaucher`
  MODIFY `id` int NOT NULL AUTO_INCREMENT;
ALTER TABLE `ai_report_types`
  MODIFY `id` int NOT NULL AUTO_INCREMENT, AUTO_INCREMENT=9;
ALTER TABLE `ai_setting`
  MODIFY `setting_id` int NOT NULL AUTO_INCREMENT, AUTO_INCREMENT=2;
ALTER TABLE `app_setting`
  MODIFY `id` int NOT NULL AUTO_INCREMENT, AUTO_INCREMENT=2;
ALTER TABLE `attendance`
  MODIFY `att_id` int NOT NULL AUTO_INCREMENT;
ALTER TABLE `bank_add`
  MODIFY `id` int NOT NULL AUTO_INCREMENT;
ALTER TABLE `closing_records`
  MODIFY `id` int NOT NULL AUTO_INCREMENT;
ALTER TABLE `currency_tbl`
  MODIFY `id` int NOT NULL AUTO_INCREMENT, AUTO_INCREMENT=3;
ALTER TABLE `customer_information`
  MODIFY `customer_id` bigint NOT NULL AUTO_INCREMENT, AUTO_INCREMENT=6;
ALTER TABLE `daily_closing`
  MODIFY `closing_id` int NOT NULL AUTO_INCREMENT;
ALTER TABLE `designation`
  MODIFY `id` int NOT NULL AUTO_INCREMENT;
ALTER TABLE `email_config`
  MODIFY `id` int NOT NULL AUTO_INCREMENT, AUTO_INCREMENT=2;
ALTER TABLE `employee_history`
  MODIFY `id` int NOT NULL AUTO_INCREMENT;
ALTER TABLE `employee_salary_payment`
  MODIFY `emp_sal_pay_id` int NOT NULL AUTO_INCREMENT;
ALTER TABLE `employee_salary_setup`
  MODIFY `e_s_s_id` int UNSIGNED NOT NULL AUTO_INCREMENT;
ALTER TABLE `expense`
  MODIFY `id` int NOT NULL AUTO_INCREMENT;
ALTER TABLE `expense_item`
  MODIFY `id` int NOT NULL AUTO_INCREMENT;
ALTER TABLE `financial_year`
  MODIFY `id` int NOT NULL AUTO_INCREMENT, AUTO_INCREMENT=3;
ALTER TABLE `gmb_salary_advance`
  MODIFY `id` int NOT NULL AUTO_INCREMENT;
ALTER TABLE `gmb_salary_generate`
  MODIFY `id` int NOT NULL AUTO_INCREMENT;
ALTER TABLE `gmb_salary_sheet_generate`
  MODIFY `ssg_id` int NOT NULL AUTO_INCREMENT;
ALTER TABLE `invoice`
  MODIFY `id` int NOT NULL AUTO_INCREMENT;
ALTER TABLE `invoice_details`
  MODIFY `id` int NOT NULL AUTO_INCREMENT;
ALTER TABLE `language`
  MODIFY `id` int UNSIGNED NOT NULL AUTO_INCREMENT, AUTO_INCREMENT=1099;
ALTER TABLE `module`
  MODIFY `id` int NOT NULL AUTO_INCREMENT, AUTO_INCREMENT=18;
ALTER TABLE `module_purchase_key`
  MODIFY `id` int NOT NULL AUTO_INCREMENT;
ALTER TABLE `payroll_tax_setup`
  MODIFY `tax_setup_id` int UNSIGNED NOT NULL AUTO_INCREMENT;
ALTER TABLE `personal_loan`
  MODIFY `per_loan_id` int NOT NULL AUTO_INCREMENT;
ALTER TABLE `person_information`
  MODIFY `id` int NOT NULL AUTO_INCREMENT;
ALTER TABLE `person_ledger`
  MODIFY `id` int NOT NULL AUTO_INCREMENT;
ALTER TABLE `pesonal_loan_information`
  MODIFY `id` int NOT NULL AUTO_INCREMENT;
ALTER TABLE `print_setting`
  MODIFY `id` int NOT NULL AUTO_INCREMENT, AUTO_INCREMENT=2;
ALTER TABLE `product_category`
  MODIFY `category_id` int NOT NULL AUTO_INCREMENT;
ALTER TABLE `product_information`
  MODIFY `id` int NOT NULL AUTO_INCREMENT;
ALTER TABLE `product_purchase`
  MODIFY `id` int NOT NULL AUTO_INCREMENT;
ALTER TABLE `product_purchase_details`
  MODIFY `id` int NOT NULL AUTO_INCREMENT;
ALTER TABLE `product_service`
  MODIFY `service_id` int NOT NULL AUTO_INCREMENT;
ALTER TABLE `quotation`
  MODIFY `id` int NOT NULL AUTO_INCREMENT;
ALTER TABLE `quotation_service_used`
  MODIFY `id` int NOT NULL AUTO_INCREMENT;
ALTER TABLE `quotation_taxinfo`
  MODIFY `id` int NOT NULL AUTO_INCREMENT;
ALTER TABLE `quot_products_used`
  MODIFY `id` int NOT NULL AUTO_INCREMENT;
ALTER TABLE `role_permission`
  MODIFY `id` int NOT NULL AUTO_INCREMENT, AUTO_INCREMENT=339;
ALTER TABLE `salary_sheet_generate`
  MODIFY `ssg_id` int UNSIGNED NOT NULL AUTO_INCREMENT;
ALTER TABLE `salary_type`
  MODIFY `salary_type_id` int NOT NULL AUTO_INCREMENT;
ALTER TABLE `sec_role`
  MODIFY `id` int NOT NULL AUTO_INCREMENT, AUTO_INCREMENT=3;
ALTER TABLE `sec_userrole`
  MODIFY `id` int NOT NULL AUTO_INCREMENT, AUTO_INCREMENT=3;
ALTER TABLE `seles_termscondi`
  MODIFY `id` int NOT NULL AUTO_INCREMENT, AUTO_INCREMENT=6;
ALTER TABLE `service_invoice`
  MODIFY `id` int NOT NULL AUTO_INCREMENT;
ALTER TABLE `service_invoice_details`
  MODIFY `id` int NOT NULL AUTO_INCREMENT;
ALTER TABLE `setting`
  MODIFY `id` int NOT NULL AUTO_INCREMENT, AUTO_INCREMENT=4;
ALTER TABLE `sms_settings`
  MODIFY `id` int NOT NULL AUTO_INCREMENT, AUTO_INCREMENT=2;
ALTER TABLE `sub_module`
  MODIFY `id` int NOT NULL AUTO_INCREMENT, AUTO_INCREMENT=146;
ALTER TABLE `supplier_information`
  MODIFY `supplier_id` bigint NOT NULL AUTO_INCREMENT;
ALTER TABLE `supplier_product`
  MODIFY `supplier_pr_id` int NOT NULL AUTO_INCREMENT;
ALTER TABLE `tax_collection`
  MODIFY `id` int NOT NULL AUTO_INCREMENT;
ALTER TABLE `tax_settings`
  MODIFY `id` int NOT NULL AUTO_INCREMENT;
ALTER TABLE `units`
  MODIFY `unit_id` int NOT NULL AUTO_INCREMENT;
ALTER TABLE `users`
  MODIFY `id` int NOT NULL AUTO_INCREMENT, AUTO_INCREMENT=8;
ALTER TABLE `user_login`
  MODIFY `id` int NOT NULL AUTO_INCREMENT, AUTO_INCREMENT=8;
ALTER TABLE `vat_tax_setting`
  MODIFY `id` int NOT NULL AUTO_INCREMENT, AUTO_INCREMENT=2;
ALTER TABLE `web_setting`
  MODIFY `setting_id` int NOT NULL AUTO_INCREMENT, AUTO_INCREMENT=2;


-- AUTO_INCREMENT definitions for all tables
ALTER TABLE `acc_coa`
  MODIFY `id` int NOT NULL AUTO_INCREMENT;
ALTER TABLE `acc_monthly_balance`
  MODIFY `id` int NOT NULL AUTO_INCREMENT;
ALTER TABLE `acc_opening_balance`
  MODIFY `id` int NOT NULL AUTO_INCREMENT;
ALTER TABLE `acc_predefine_account`
  MODIFY `id` int NOT NULL AUTO_INCREMENT;
ALTER TABLE `acc_subcode`
  MODIFY `id` int NOT NULL AUTO_INCREMENT;
ALTER TABLE `acc_transaction`
  MODIFY `ID` int NOT NULL AUTO_INCREMENT;
ALTER TABLE `acc_vaucher`
  MODIFY `id` int NOT NULL AUTO_INCREMENT;
ALTER TABLE `ai_report_types`
  MODIFY `id` int NOT NULL AUTO_INCREMENT;
ALTER TABLE `ai_setting`
  MODIFY `setting_id` int NOT NULL AUTO_INCREMENT;
ALTER TABLE `app_setting`
  MODIFY `id` int NOT NULL AUTO_INCREMENT;
ALTER TABLE `attendance`
  MODIFY `att_id` int NOT NULL AUTO_INCREMENT;
ALTER TABLE `bank_add`
  MODIFY `id` int NOT NULL AUTO_INCREMENT;
ALTER TABLE `closing_records`
  MODIFY `id` int NOT NULL AUTO_INCREMENT;
ALTER TABLE `currency_tbl`
  MODIFY `id` int NOT NULL AUTO_INCREMENT;
ALTER TABLE `customer_information`
  MODIFY `customer_id` bigint NOT NULL AUTO_INCREMENT;
ALTER TABLE `daily_closing`
  MODIFY `closing_id` int NOT NULL AUTO_INCREMENT;
ALTER TABLE `designation`
  MODIFY `id` int NOT NULL AUTO_INCREMENT;
ALTER TABLE `email_config`
  MODIFY `id` int NOT NULL AUTO_INCREMENT;
ALTER TABLE `employee_history`
  MODIFY `id` int NOT NULL AUTO_INCREMENT;
ALTER TABLE `employee_salary_payment`
  MODIFY `emp_sal_pay_id` int NOT NULL AUTO_INCREMENT;
ALTER TABLE `employee_salary_setup`
  MODIFY `e_s_s_id` int UNSIGNED NOT NULL AUTO_INCREMENT;
ALTER TABLE `expense`
  MODIFY `id` int NOT NULL AUTO_INCREMENT;
ALTER TABLE `expense_item`
  MODIFY `id` int NOT NULL AUTO_INCREMENT;
ALTER TABLE `financial_year`
  MODIFY `id` int NOT NULL AUTO_INCREMENT;
ALTER TABLE `gmb_salary_advance`
  MODIFY `id` int NOT NULL AUTO_INCREMENT;
ALTER TABLE `gmb_salary_generate`
  MODIFY `id` int NOT NULL AUTO_INCREMENT;
ALTER TABLE `gmb_salary_sheet_generate`
  MODIFY `ssg_id` int NOT NULL AUTO_INCREMENT;
ALTER TABLE `invoice`
  MODIFY `id` int NOT NULL AUTO_INCREMENT;
ALTER TABLE `invoice_details`
  MODIFY `id` int NOT NULL AUTO_INCREMENT;
ALTER TABLE `language`
  MODIFY `id` int UNSIGNED NOT NULL AUTO_INCREMENT;
ALTER TABLE `module`
  MODIFY `id` int NOT NULL AUTO_INCREMENT;
ALTER TABLE `module_purchase_key`
  MODIFY `id` int NOT NULL AUTO_INCREMENT;
ALTER TABLE `payroll_tax_setup`
  MODIFY `tax_setup_id` int UNSIGNED NOT NULL AUTO_INCREMENT;
ALTER TABLE `personal_loan`
  MODIFY `per_loan_id` int NOT NULL AUTO_INCREMENT;
ALTER TABLE `person_information`
  MODIFY `id` int NOT NULL AUTO_INCREMENT;
ALTER TABLE `person_ledger`
  MODIFY `id` int NOT NULL AUTO_INCREMENT;
ALTER TABLE `pesonal_loan_information`
  MODIFY `id` int NOT NULL AUTO_INCREMENT;
ALTER TABLE `print_setting`
  MODIFY `id` int NOT NULL AUTO_INCREMENT;
ALTER TABLE `product_category`
  MODIFY `category_id` int NOT NULL AUTO_INCREMENT;
ALTER TABLE `product_information`
  MODIFY `id` int NOT NULL AUTO_INCREMENT;
ALTER TABLE `product_purchase`
  MODIFY `id` int NOT NULL AUTO_INCREMENT;
ALTER TABLE `product_purchase_details`
  MODIFY `id` int NOT NULL AUTO_INCREMENT;
ALTER TABLE `product_service`
  MODIFY `service_id` int NOT NULL AUTO_INCREMENT;
ALTER TABLE `quotation`
  MODIFY `id` int NOT NULL AUTO_INCREMENT;
ALTER TABLE `quotation_service_used`
  MODIFY `id` int NOT NULL AUTO_INCREMENT;
ALTER TABLE `quotation_taxinfo`
  MODIFY `id` int NOT NULL AUTO_INCREMENT;
ALTER TABLE `quot_products_used`
  MODIFY `id` int NOT NULL AUTO_INCREMENT;
ALTER TABLE `role_permission`
  MODIFY `id` int NOT NULL AUTO_INCREMENT;
ALTER TABLE `salary_sheet_generate`
  MODIFY `ssg_id` int UNSIGNED NOT NULL AUTO_INCREMENT;
ALTER TABLE `salary_type`
  MODIFY `salary_type_id` int NOT NULL AUTO_INCREMENT;
ALTER TABLE `sec_role`
  MODIFY `id` int NOT NULL AUTO_INCREMENT;
ALTER TABLE `sec_userrole`
  MODIFY `id` int NOT NULL AUTO_INCREMENT;
ALTER TABLE `seles_termscondi`
  MODIFY `id` int NOT NULL AUTO_INCREMENT;
ALTER TABLE `service_invoice`
  MODIFY `id` int NOT NULL AUTO_INCREMENT;
ALTER TABLE `service_invoice_details`
  MODIFY `id` int NOT NULL AUTO_INCREMENT;
ALTER TABLE `setting`
  MODIFY `id` int NOT NULL AUTO_INCREMENT;
ALTER TABLE `sms_settings`
  MODIFY `id` int NOT NULL AUTO_INCREMENT;
ALTER TABLE `sub_module`
  MODIFY `id` int NOT NULL AUTO_INCREMENT;
ALTER TABLE `supplier_information`
  MODIFY `supplier_id` bigint NOT NULL AUTO_INCREMENT;
ALTER TABLE `supplier_product`
  MODIFY `supplier_pr_id` int NOT NULL AUTO_INCREMENT;
ALTER TABLE `tax_collection`
  MODIFY `id` int NOT NULL AUTO_INCREMENT;
ALTER TABLE `tax_settings`
  MODIFY `id` int NOT NULL AUTO_INCREMENT;
ALTER TABLE `units`
  MODIFY `unit_id` int NOT NULL AUTO_INCREMENT;
ALTER TABLE `users`
  MODIFY `id` int NOT NULL AUTO_INCREMENT;
ALTER TABLE `user_login`
  MODIFY `id` int NOT NULL AUTO_INCREMENT;
ALTER TABLE `vat_tax_setting`
  MODIFY `id` int NOT NULL AUTO_INCREMENT;
ALTER TABLE `web_setting`
  MODIFY `setting_id` int NOT NULL AUTO_INCREMENT;

COMMIT;
